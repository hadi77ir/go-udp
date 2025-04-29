package types

import (
	"io"
	"net"
	"time"
)

type PacketConn interface {
	io.Closer
	Read(p []byte, oob []byte) (dataN int, oobN int, ecn ECN, err error)
	Write(p []byte, oob []byte, gsoSize int, ecn ECN) (dataN int, oobN int, err error)
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	Capabilities() ConnCapabilities
}

type SuperConn interface {
	io.Closer
	Accept() (PacketConn, error)
	Addr() net.Addr
	SetDeadline(t time.Time) error
	Capabilities() ConnCapabilities
}

type GetSubConnFunc func(raddr net.Addr) (PacketConn, error)
