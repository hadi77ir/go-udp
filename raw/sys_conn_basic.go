package raw

import (
	"errors"
	"net"

	"github.com/hadi77ir/go-logging"
	"github.com/hadi77ir/go-udp/log"
	"github.com/hadi77ir/go-udp/types"
)

// The basicConn is the most trivial implementation of a RawConn.
// It reads a single packet from the underlying net.PacketConn.
// It is used when
// * the net.PacketConn is not a OOBCapablePacketConn, and
// * when the OS doesn't support OOB.
type BasicConn struct {
	net.PacketConn
	isConnected bool
	supportsDF  bool
}

var _ types.RawConn = &BasicConn{}

func (c *BasicConn) RemoteAddr() net.Addr {
	if cc, ok := c.PacketConn.(remoteAddrSock); ok {
		return cc.RemoteAddr()
	}
	return nil
}

func (c *BasicConn) ReadPacket(b []byte, oob []byte) (bytesRead int, oobRead int, ecn types.ECN, remoteAddr net.Addr, err error) {
	if cc, ok := c.PacketConn.(connectedConn); c.isConnected && ok {
		bytesRead, err = cc.Read(b)
	} else {
		bytesRead, remoteAddr, err = c.PacketConn.ReadFrom(b)
	}
	if err != nil {
		return 0, 0, types.ECNUnsupported, nil, err
	}
	return
}

func (c *BasicConn) WritePacket(b []byte, oob []byte, gsoSize uint16, ecn types.ECN, addr net.Addr) (n int, oobN int, err error) {
	if gsoSize != 0 {
		// exits
		log.Log(logging.PanicLevel, "cannot use GSO with a basicConn")
		// failsafe
		return 0, 0, errors.New("GSO not supported")
	}
	if ecn != types.ECNUnsupported {
		// exits
		log.Log(logging.PanicLevel, "cannot use ECN with a basicConn")
		// failsafe
		return 0, 0, errors.New("ECN not supported")
	}

	if c.isConnected {
		if cc, ok := c.PacketConn.(connectedConn); ok {
			n, err = cc.Write(b)
			return
		} else {
			addr = nil
		}
	}
	n, err = c.PacketConn.WriteTo(b, addr)
	return
}

func (c *BasicConn) Capabilities() types.ConnCapabilities {
	return types.NewConnCapabilities(c.supportsDF, false, false)
}
