package udp

import (
	"errors"
	"github.com/hadi77ir/go-udp/types"
	"net"
	"time"
)

type SingleAddrConn struct {
	base types.RawConn
	addr net.Addr
}

func (c *SingleAddrConn) Close() error {
	return c.base.Close()
}

func (c *SingleAddrConn) Read(p []byte, oob []byte) (dataN int, oobN int, ecn types.ECN, err error) {
	var addr net.Addr
	dataN, oobN, ecn, addr, err = c.base.ReadPacket(p, oob)
	if addr != nil && c.addr != nil {
		// check if addr is the same or the packet should be ignored
		if addr.String() != c.addr.String() {
			// ignore this packet
			return 0, 0, 0, nil
		}
	}
	return
}

func (c *SingleAddrConn) Write(p []byte, oob []byte, gsoSize int, ecn types.ECN) (dataN int, oobN int, err error) {
	dataN, oobN, err = c.base.WritePacket(p, oob, uint16(gsoSize), ecn, c.addr)
	return
}

func (c *SingleAddrConn) LocalAddr() net.Addr {
	return c.base.LocalAddr()
}

func (c *SingleAddrConn) RemoteAddr() net.Addr {
	addr := c.base.RemoteAddr()
	if addr != nil {
		return addr
	}
	return c.addr
}

func (c *SingleAddrConn) SetDeadline(t time.Time) error {
	return c.SetReadDeadline(t)
}

func (c *SingleAddrConn) SetReadDeadline(t time.Time) error {
	return c.base.SetReadDeadline(t)
}

func (c *SingleAddrConn) SetWriteDeadline(t time.Time) error {
	return errors.ErrUnsupported
}

func (c *SingleAddrConn) Capabilities() types.ConnCapabilities {
	return c.base.Capabilities()
}

var _ types.PacketConn = &SingleAddrConn{}
