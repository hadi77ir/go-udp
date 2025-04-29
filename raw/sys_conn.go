package raw

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hadi77ir/go-logging"

	"github.com/hadi77ir/go-udp/log"
	"github.com/hadi77ir/go-udp/types"
)

var setBufferWarningOnce sync.Once

// OOBCapablePacketConn is a connection that allows the reading of ECN bits from the IP header.
// If the PacketConn passed to Dial or Listen satisfies this interface, quic-go will use it.
// In this case, ReadMsgUDP() will be used instead of ReadFrom() to read packets.
type OOBCapablePacketConn interface {
	net.PacketConn
	SyscallConn() (syscall.RawConn, error)
	SetReadBuffer(int) error
	ReadMsgUDP(b, oob []byte) (n, oobn, flags int, addr *net.UDPAddr, err error)
	WriteMsgUDP(b, oob []byte, addr *net.UDPAddr) (n, oobn int, err error)
}

var _ OOBCapablePacketConn = &net.UDPConn{}

type remoteAddrSock interface {
	RemoteAddr() net.Addr
}

var _ remoteAddrSock = &net.UDPConn{}

type connectedConn interface {
	io.ReadWriteCloser
}

var _ connectedConn = &net.UDPConn{}

// WrapConn wraps given connection into a RawConn, allowing for sending OOB data (ECN bits)
// To disable batch writing that involves buffering and interval-based flushing, set writeInterval to zero.
func WrapConn(pc net.PacketConn, writeInterval time.Duration) (types.RawConn, error) {
	if err := setReceiveBuffer(pc); err != nil {
		if !strings.Contains(err.Error(), "use of closed network connection") {
			setBufferWarningOnce.Do(func() {
				if disable, _ := strconv.ParseBool(os.Getenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING")); disable {
					return
				}
				log.Log(logging.WarnLevel, fmt.Sprintf("%s. See https://github.com/quic-go/quic-go/wiki/UDP-Buffer-Sizes for details.", err))
			})
		}
	}
	if err := setSendBuffer(pc); err != nil {
		if !strings.Contains(err.Error(), "use of closed network connection") {
			setBufferWarningOnce.Do(func() {
				if disable, _ := strconv.ParseBool(os.Getenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING")); disable {
					return
				}
				log.Log(logging.WarnLevel, fmt.Sprintf("%s. See https://github.com/quic-go/quic-go/wiki/UDP-Buffer-Sizes for details.", err))
			})
		}
	}

	conn, ok := pc.(interface {
		SyscallConn() (syscall.RawConn, error)
	})
	var supportsDF bool
	if ok {
		rawConn, err := conn.SyscallConn()
		if err != nil {
			return nil, err
		}

		// only set DF on UDP sockets
		if _, ok := pc.LocalAddr().(*net.UDPAddr); ok {
			var err error
			supportsDF, err = setDF(rawConn)
			if err != nil {
				return nil, err
			}
		}
	}
	c, ok := pc.(OOBCapablePacketConn)
	if !ok {
		log.Log(logging.InfoLevel, "PacketConn is not a net.UDPConn or OOB-capable connection. Disabling optimizations possible on UDP connections.")
		return &BasicConn{PacketConn: pc, supportsDF: supportsDF}, nil
	}
	return newConn(c, supportsDF, writeInterval)
}
