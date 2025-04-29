package udp

import (
	"errors"
	"github.com/hadi77ir/go-udp/types"
	"net"
	"time"
)

type ConnectedConn struct {
	base types.RawConn
}

func (c *ConnectedConn) Close() error {
	return c.base.Close()
}

func (c *ConnectedConn) Read(p []byte, oob []byte) (dataN int, oobN int, ecn types.ECN, err error) {
	dataN, oobN, ecn, _, err = c.base.ReadPacket(p, oob)
	return
}

func (c *ConnectedConn) Write(p []byte, oob []byte, gsoSize int, ecn types.ECN) (dataN int, oobN int, err error) {
	dataN, oobN, err = c.base.WritePacket(p, oob, uint16(gsoSize), ecn, nil)
	return
}

func (c *ConnectedConn) LocalAddr() net.Addr {
	return c.base.LocalAddr()
}

func (c *ConnectedConn) RemoteAddr() net.Addr {
	return c.base.RemoteAddr()
}

func (c *ConnectedConn) SetDeadline(t time.Time) error {
	return c.SetReadDeadline(t)
}

func (c *ConnectedConn) SetReadDeadline(t time.Time) error {
	return c.base.SetReadDeadline(t)
}

func (c *ConnectedConn) SetWriteDeadline(t time.Time) error {
	return errors.ErrUnsupported
}

var _ types.PacketConn = &ConnectedConn{}
