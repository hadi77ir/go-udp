//go:build darwin || linux || freebsd

package raw

import (
	"fmt"
	"net"
	"testing"
	"time"

	"golang.org/x/net/ipv4"
	"golang.org/x/sys/unix"

	"github.com/stretchr/testify/require"

	"github.com/hadi77ir/go-udp/types"
)

func isIPv4(ip net.IP) bool { return ip.To4() != nil }

type receivedPacket struct {
	buf        []byte
	oob        []byte
	ecn        types.ECN
	remoteAddr net.Addr
}

func runSysConnServer(t *testing.T, network string, addr *net.UDPAddr) (*net.UDPAddr, <-chan receivedPacket) {
	t.Helper()
	udpConn, err := net.ListenUDP(network, addr)
	require.NoError(t, err)
	t.Cleanup(func() { udpConn.Close() })

	oobConn, err := newConn(udpConn, true, 0)
	require.NoError(t, err)
	require.True(t, oobConn.Capabilities().DF())

	packetChan := make(chan receivedPacket, 1)
	go func() {
		for {
			pkt := receivedPacket{buf: make([]byte, 1024), oob: make([]byte, 1024), ecn: types.ECNNon}
			dataN, oobN, ecn, raddr, err := oobConn.ReadPacket(pkt.buf, pkt.oob)
			if err != nil {
				return
			}
			pkt.ecn = ecn
			pkt.buf = pkt.buf[:dataN]
			pkt.oob = pkt.oob[:oobN]
			pkt.remoteAddr = raddr
			packetChan <- pkt
		}
	}()
	return udpConn.LocalAddr().(*net.UDPAddr), packetChan
}

// sendUDPPacketWithECN opens a new UDP socket and sends one packet with the ECN set.
// It returns the local address of the socket.
func sendUDPPacketWithECN(t *testing.T, network string, addr *net.UDPAddr, setECN func(uintptr)) net.Addr {
	conn, err := net.DialUDP(network, nil, addr)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	rawConn, err := conn.SyscallConn()
	require.NoError(t, err)
	require.NoError(t, rawConn.Control(func(fd uintptr) { setECN(fd) }))
	_, err = conn.Write([]byte("foobar"))
	require.NoError(t, err)
	return conn.LocalAddr()
}

func TestReadECNFlagsIPv4(t *testing.T) {
	addr, packetChan := runSysConnServer(t, "udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})

	sentFrom := sendUDPPacketWithECN(t,
		"udp4",
		addr,
		func(fd uintptr) {
			require.NoError(t, unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_TOS, 2))
		},
	)

	select {
	case p := <-packetChan:
		require.Equal(t, []byte("foobar"), p.buf)
		require.Equal(t, sentFrom, p.remoteAddr)
		require.Equal(t, types.ECT0, p.ecn)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for packet")
	}
}

func TestReadECNFlagsIPv6(t *testing.T) {
	addr, packetChan := runSysConnServer(t, "udp6", &net.UDPAddr{IP: net.IPv6loopback, Port: 0})

	sentFrom := sendUDPPacketWithECN(t,
		"udp6",
		addr,
		func(fd uintptr) {
			require.NoError(t, unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_TCLASS, 3))
		},
	)

	select {
	case p := <-packetChan:
		require.Equal(t, []byte("foobar"), p.buf)
		require.Equal(t, sentFrom, p.remoteAddr)
		require.Equal(t, types.ECNCE, p.ecn)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for packet")
	}
}

func TestReadECNFlagsDualStack(t *testing.T) {
	addr, packetChan := runSysConnServer(t, "udp", &net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: 0})

	// IPv4
	sentFrom := sendUDPPacketWithECN(t,
		"udp4",
		&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: addr.Port},
		func(fd uintptr) {
			require.NoError(t, unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_TOS, 3))
		},
	)

	select {
	case p := <-packetChan:
		require.True(t, isIPv4(p.remoteAddr.(*net.UDPAddr).IP))
		require.Equal(t, sentFrom.String(), p.remoteAddr.String())
		require.Equal(t, types.ECNCE, p.ecn)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for packet")
	}

	// IPv6
	sentFrom = sendUDPPacketWithECN(t,
		"udp6",
		&net.UDPAddr{IP: net.IPv6loopback, Port: addr.Port},
		func(fd uintptr) {
			require.NoError(t, unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_TCLASS, 1))
		},
	)

	select {
	case p := <-packetChan:
		require.Equal(t, sentFrom, p.remoteAddr)
		require.False(t, isIPv4(p.remoteAddr.(*net.UDPAddr).IP))
		require.Equal(t, types.ECT1, p.ecn)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for packet")
	}
}

func TestSendPacketsWithECNOnIPv4(t *testing.T) {
	addr, packetChan := runSysConnServer(t, "udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})

	c, err := net.ListenUDP("udp4", nil)
	require.NoError(t, err)
	defer c.Close()

	for _, val := range []types.ECN{types.ECNNon, types.ECT1, types.ECT0, types.ECNCE} {
		_, _, err = c.WriteMsgUDP([]byte("foobar"), appendIPv4ECNMsg([]byte{}, val), addr)
		require.NoError(t, err)
		select {
		case p := <-packetChan:
			require.Equal(t, []byte("foobar"), p.buf)
			require.Equal(t, val, p.ecn)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for packet")
		}
	}
}

func TestSendPacketsWithECNOnIPv6(t *testing.T) {
	addr, packetChan := runSysConnServer(t, "udp6", &net.UDPAddr{IP: net.IPv6loopback, Port: 0})

	c, err := net.ListenUDP("udp6", nil)
	require.NoError(t, err)
	defer c.Close()

	for _, val := range []types.ECN{types.ECNNon, types.ECT1, types.ECT0, types.ECNCE} {
		_, _, err = c.WriteMsgUDP([]byte("foobar"), appendIPv6ECNMsg([]byte{}, val), addr)
		require.NoError(t, err)
		select {
		case p := <-packetChan:
			require.Equal(t, []byte("foobar"), p.buf)
			require.Equal(t, val, p.ecn)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for packet")
		}
	}
}

type oobRecordingConn struct {
	*net.UDPConn
	oobs [][]byte
}

func (c *oobRecordingConn) WriteMsgUDP(b, oob []byte, addr *net.UDPAddr) (n, oobn int, err error) {
	c.oobs = append(c.oobs, oob)
	return c.UDPConn.WriteMsgUDP(b, oob, addr)
}

type mockBatchConn struct {
	t          *testing.T
	numMsgRead int

	callCounter int
}

var _ sysBatchReader = &mockBatchConn{}

func (c *mockBatchConn) Close() error {
	// nothing
	return nil
}

func (c *mockBatchConn) ReadBatch(ms []ipv4.Message, _ int) (int, error) {
	require.Len(c.t, ms, batchSize)
	for i := 0; i < c.numMsgRead; i++ {
		require.Len(c.t, ms[i].Buffers, 1)
		require.Len(c.t, ms[i].Buffers[0], MaxPacketBufferSize)
		data := []byte(fmt.Sprintf("message %d", c.callCounter*c.numMsgRead+i))
		ms[i].Buffers[0] = data
		ms[i].N = len(data)
	}
	c.callCounter++
	return c.numMsgRead, nil
}

func newUDPConnLocalhost(t testing.TB) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestReadsMultipleMessagesInOneBatch(t *testing.T) {
	bc := &mockBatchConn{t: t, numMsgRead: batchSize/2 + 1}

	udpConn := newUDPConnLocalhost(t)
	oobConn, err := newConn(udpConn, true, 0)
	require.NoError(t, err)
	oobConn.reader = newBatchReader(bc, nil)

	buf := make([]byte, MaxPacketBufferSize)

	for i := 0; i < batchSize+1; i++ {
		n, _, _, _, err := oobConn.ReadPacket(buf, []byte{})
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("message %d", i), string(buf[:n]))
	}
	require.Equal(t, 2, bc.callCounter)
}

// Only if appendUDPSegmentSizeMsg actually appends a message (and isn't only a stub implementation),
// GSO is actually supported on this platform.
var platformSupportsGSO = len(appendUDPSegmentSizeMsg([]byte{}, 1337)) > 0

func TestSysConnSendGSO(t *testing.T) {
	if !platformSupportsGSO {
		t.Skip("GSO not supported on this platform")
	}

	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	c := &oobRecordingConn{UDPConn: udpConn}
	oobConn, err := newConn(c, true, 0)
	require.NoError(t, err)
	require.True(t, oobConn.Capabilities().GSO())

	oob := make([]byte, 0, 123)
	oobConn.WritePacket([]byte("foobar"), oob, 3, types.ECNCE, udpConn.LocalAddr())
	require.Len(t, c.oobs, 1)
	oobMsg := c.oobs[0]
	require.NotEmpty(t, oobMsg)
	require.Equal(t, cap(oob), cap(oobMsg)) // check that it appended to oob
	expected := appendUDPSegmentSizeMsg([]byte{}, 3)
	// Check that the first control message is the OOB control message.
	require.Equal(t, expected, oobMsg[:len(expected)])
}
