//go:build darwin || linux || freebsd

package raw

import (
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"golang.org/x/sys/unix"

	"github.com/hadi77ir/go-logging"
	"github.com/hadi77ir/go-udp/log"
	"github.com/hadi77ir/go-udp/types"
)

// MaxPacketBufferSize maximum packet size of any QUIC packet, based on
// ethernet's max size, minus the IP and UDP headers. IPv6 has a 40 byte header,
// UDP adds another 8 bytes.  This is a total overhead of 48 bytes.
// Ethernet's max packet size is 1500 bytes,  1500 - 48 = 1452.
const MaxPacketBufferSize = 1452

const (
	ecnMask       = 0x3
	oobBufferSize = 128
)

var ErrClosed = io.EOF

// Contrary to what the naming suggests, the ipv{4,6}.Message is not dependent on the IP version.
// They're both just aliases for x/net/internal/socket.Message.
// This means we can use this struct to read from a socket that receives both IPv4 and IPv6 messages.
var _ ipv4.Message = ipv6.Message{}

type sysBatchReader interface {
	io.Closer
	ReadBatch(ms []ipv4.Message, flags int) (int, error)
}

type sysBatchWriter interface {
	io.Closer
	WriteBatch(ms []ipv4.Message, flags int) (int, error)
}

func InspectReadBuffer(c syscall.RawConn) (int, error) {
	var size int
	var serr error
	if err := c.Control(func(fd uintptr) {
		size, serr = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_RCVBUF)
	}); err != nil {
		return 0, err
	}
	return size, serr
}

func InspectWriteBuffer(c syscall.RawConn) (int, error) {
	var size int
	var serr error
	if err := c.Control(func(fd uintptr) {
		size, serr = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_SNDBUF)
	}); err != nil {
		return 0, err
	}
	return size, serr
}

func IsECNDisabledUsingEnv() bool {
	disabled, err := strconv.ParseBool(os.Getenv("QUIC_GO_DISABLE_ECN"))
	return err == nil && disabled
}

type oobConn struct {
	OOBCapablePacketConn
	reader oobReader
	writer oobWriter

	cap types.ConnCapabilities
}

var _ types.RawConn = &oobConn{}

func newConn(c OOBCapablePacketConn, supportsDF bool, writeInterval time.Duration) (*oobConn, error) {
	rawConn, err := c.SyscallConn()
	if err != nil {
		return nil, err
	}
	var needsPacketInfo bool
	if udpAddr, ok := c.LocalAddr().(*net.UDPAddr); ok && udpAddr.IP.IsUnspecified() {
		needsPacketInfo = true
	}
	// We don't know if this a IPv4-only, IPv6-only or a IPv4-and-IPv6 connection.
	// Try enabling receiving of ECN and packet info for both IP versions.
	// We expect at least one of those syscalls to succeed.
	var errECNIPv4, errECNIPv6, errPIIPv4, errPIIPv6 error
	if err := rawConn.Control(func(fd uintptr) {
		errECNIPv4 = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_RECVTOS, 1)
		errECNIPv6 = unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_RECVTCLASS, 1)

		if needsPacketInfo {
			errPIIPv4 = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, ipv4PKTINFO, 1)
			errPIIPv6 = unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_RECVPKTINFO, 1)
		}
	}); err != nil {
		return nil, err
	}
	switch {
	case errECNIPv4 == nil && errECNIPv6 == nil:
		log.Log(logging.DebugLevel, "Activating reading of ECN bits for IPv4 and IPv6.")
	case errECNIPv4 == nil && errECNIPv6 != nil:
		log.Log(logging.DebugLevel, "Activating reading of ECN bits for IPv4.")
	case errECNIPv4 != nil && errECNIPv6 == nil:
		log.Log(logging.DebugLevel, "Activating reading of ECN bits for IPv6.")
	case errECNIPv4 != nil && errECNIPv6 != nil:
		// todo: refactor
		return nil, errors.New("activating ECN failed for both IPv4 and IPv6")
	}
	if needsPacketInfo {
		switch {
		case errPIIPv4 == nil && errPIIPv6 == nil:
			log.Log(logging.DebugLevel, "Activating reading of packet info for IPv4 and IPv6.")
		case errPIIPv4 == nil && errPIIPv6 != nil:
			log.Log(logging.DebugLevel, "Activating reading of packet info bits for IPv4.")
		case errPIIPv4 != nil && errPIIPv6 == nil:
			log.Log(logging.DebugLevel, "Activating reading of packet info bits for IPv6.")
		case errPIIPv4 != nil && errPIIPv6 != nil:
			// todo: refactor
			return nil, errors.New("activating packet info failed for both IPv4 and IPv6")
		}
	}

	// Check if this socket is a "connected" socket.
	isConnected := false
	if ra, ok := c.(remoteAddrSock); ok && ra.RemoteAddr() != nil {
		// this is a connected socket. use nil remote address.
		isConnected = true
	}

	// Allows callers to pass in a connection that already satisfies batchReader interface
	// to make use of the optimisation. Otherwise, ipv4.NewPacketConn would unwrap the file descriptor
	// via SyscallConn(), and read it that way, which might not be what the caller wants.
	var br sysBatchReader
	if ibr, ok := c.(sysBatchReader); ok {
		br = ibr
	} else {
		br = ipv4.NewPacketConn(c)
	}

	// Same, but for batch writers.
	var bw sysBatchWriter
	if ibw, ok := c.(sysBatchWriter); ok {
		bw = ibw
	} else if ibw, ok := br.(sysBatchWriter); ok {
		bw = ibw
	} else {
		bw = ipv4.NewPacketConn(c)
	}

	reader := newBatchReader(br, nil)
	var writer oobWriter
	if writeInterval > 0 {
		writer = newBatchWriter(bw, isConnected, writeInterval, nil)
	} else {
		writer = &basicWriter{OOBCapablePacketConn: c, isConnected: isConnected}
	}

	oobConn := &oobConn{
		OOBCapablePacketConn: c,
		reader:               reader,
		writer:               writer,
		cap: types.NewConnCapabilities(
			supportsDF,
			IsGSOEnabled(rawConn),
			IsECNEnabled()),
	}
	return oobConn, nil
}

var invalidCmsgOnceV4, invalidCmsgOnceV6 sync.Once

func (c *oobConn) ReadPacket(b []byte, oob []byte) (bytesRead int, oobRead int, ecn types.ECN, remoteAddr net.Addr, err error) {
	bytesRead, oobRead, remoteAddr, err = c.reader.Read(b, oob)
	// check for errors
	if err != nil {
		// todo: if err is closed, close and release allocated resources
		return
	}
	oobData := oob[:oobRead]
	for len(oobData) > 0 {
		hdr, body, remainder, err := unix.ParseOneSocketControlMessage(oobData)
		if err != nil {
			return 0, 0, types.ECNNon, nil, err
		}
		if hdr.Level == unix.IPPROTO_IP {
			switch hdr.Type {
			case msgTypeIPTOS:
				ecn = types.ParseECNHeaderBits(body[0] & ecnMask)
				// todo: check if it works without this block
				//case ipv4PKTINFO:
				//	// struct in_pktinfo {
				//	// 	unsigned int   ipi_ifindex;  /* Interface index */
				//	// 	struct in_addr ipi_spec_dst; /* Local address */
				//	// 	struct in_addr ipi_addr;     /* Header Destination address */
				//	// };
				//	if len(body) == 12 {
				//		// todo: cache remoteaddr to reduce allocations
				//		// todo: IP must be copied to a new bytes slice
				//		remoteAddr = &net.UDPAddr{IP: body[8:12], Port: remoteAddr.(*net.UDPAddr).Port}
				//	} else {
				//		invalidCmsgOnceV4.Do(func() {
				//			Log(logging.WarnLevel, fmt.Sprintf("Received invalid IPv4 packet info control message: %+x. "+
				//				"This should never occur, please open a new issue and include details about the architecture.", body))
				//		})
				//	}
			}
		}
		if hdr.Level == unix.IPPROTO_IPV6 {
			switch hdr.Type {
			case unix.IPV6_TCLASS:
				ecn = types.ParseECNHeaderBits(body[0] & ecnMask)
				// todo: check if it works without this block
				//case unix.IPV6_PKTINFO:
				//	// struct in6_pktinfo {
				//	// 	struct in6_addr ipi6_addr;    /* src/dst IPv6 address */
				//	// 	unsigned int    ipi6_ifindex; /* send/recv interface index */
				//	// };
				//	if len(body) == 20 {
				//		// todo: cache remoteaddr
				//		// todo: IP must be copied to a new byte slice
				//		remoteAddr = &net.UDPAddr{IP: body[:16], Port: remoteAddr.(*net.UDPAddr).Port}
				//	} else {
				//		invalidCmsgOnceV6.Do(func() {
				//			Log(logging.WarnLevel, fmt.Sprintf("Received invalid IPv6 packet info control message: %+x. "+
				//				"This should never occur, please open a new issue and include details about the architecture.", body))
				//		})
				//	}
			}
		}
		oobData = remainder
	}
	return
}

func (c *oobConn) RemoteAddr() net.Addr {
	if cc, ok := c.OOBCapablePacketConn.(remoteAddrSock); ok {
		return cc.RemoteAddr()
	}
	return nil
}

// WritePacket writes a new packet.
func (c *oobConn) WritePacket(b []byte, packetInfoOOB []byte, gsoSize uint16, ecn types.ECN, addr net.Addr) (n int, oobN int, err error) {
	oob := packetInfoOOB
	if gsoSize > 0 {
		if !c.Capabilities().GSO() {
			// exits
			log.Log(logging.PanicLevel, "GSO disabled")
			// failsafe
			return 0, 0, errors.New("GSO disabled")
		}
		oob = appendUDPSegmentSizeMsg(oob, gsoSize)
	}
	if ecn != types.ECNUnsupported {
		if !c.Capabilities().ECN() {
			// exits
			log.Log(logging.PanicLevel, "tried to send an ECN-marked packet although ECN is disabled")
			// failsafe
			return 0, 0, errors.New("ECN is disabled")
		}
		if remoteUDPAddr, ok := addr.(*net.UDPAddr); ok {
			if remoteUDPAddr.IP.To4() != nil {
				oob = appendIPv4ECNMsg(oob, ecn)
			} else {
				oob = appendIPv6ECNMsg(oob, ecn)
			}
		}
	}

	n, oobN, err = c.writer.Write(b, oob, addr)
	// check for errors
	if err != nil {
		// todo: if err is closed, close and release allocated resources
		return 0, 0, err
	}
	return
}

func (c *oobConn) Close() error {
	err := c.OOBCapablePacketConn.Close()
	// release all buffers
	_ = c.reader.Close()
	_ = c.writer.Close()
	return err
}

func (c *oobConn) Capabilities() types.ConnCapabilities {
	return c.cap
}

func OOBForAddr(addrInfo netip.Addr) []byte {
	if addrInfo.Is4() {
		ip := addrInfo.As4()
		// struct in_pktinfo {
		// 	unsigned int   ipi_ifindex;  /* Interface index */
		// 	struct in_addr ipi_spec_dst; /* Local address */
		// 	struct in_addr ipi_addr;     /* Header Destination address */
		// };
		cm := ipv4.ControlMessage{
			Src: ip[:],
		}
		return cm.Marshal()
	} else if addrInfo.Is6() {
		ip := addrInfo.As16()
		// struct in6_pktinfo {
		// 	struct in6_addr ipi6_addr;    /* src/dst IPv6 address */
		// 	unsigned int    ipi6_ifindex; /* send/recv interface index */
		// };
		cm := ipv6.ControlMessage{
			Src: ip[:],
		}
		return cm.Marshal()
	}
	return nil
}

func appendIPv4ECNMsg(b []byte, val types.ECN) []byte {
	startLen := len(b)
	b = append(b, make([]byte, unix.CmsgSpace(ECNIPv4DataLen))...)
	h := (*unix.Cmsghdr)(unsafe.Pointer(&b[startLen]))
	h.Level = syscall.IPPROTO_IP
	h.Type = unix.IP_TOS
	h.SetLen(unix.CmsgLen(ECNIPv4DataLen))

	// UnixRights uses the private `data` method, but I *think* this achieves the same goal.
	offset := startLen + unix.CmsgSpace(0)
	b[offset] = val.ToHeaderBits()
	return b
}

func appendIPv6ECNMsg(b []byte, val types.ECN) []byte {
	startLen := len(b)
	const dataLen = 4
	b = append(b, make([]byte, unix.CmsgSpace(dataLen))...)
	h := (*unix.Cmsghdr)(unsafe.Pointer(&b[startLen]))
	h.Level = syscall.IPPROTO_IPV6
	h.Type = unix.IPV6_TCLASS
	h.SetLen(unix.CmsgLen(dataLen))

	// UnixRights uses the private `data` method, but I *think* this achieves the same goal.
	offset := startLen + unix.CmsgSpace(0)
	b[offset] = val.ToHeaderBits()
	return b
}
