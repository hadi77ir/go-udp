package udp

import (
	"net"
	"time"

	"github.com/hadi77ir/go-udp/raw"
	"github.com/hadi77ir/go-udp/types"
)

// ConnConfig stores options for dialing an address or received connections.
type ConnConfig struct {
	// ReadBufferSize sets the size of the operating system's
	// receive buffer associated with the dialed connection.
	// The only time this is effective is when a super-connection
	// is being created.
	ReadBufferSize int

	// WriteBufferSize sets the size of the operating system's
	// send buffer associated with the connection.
	// The only time this is effective is when a super-connection
	// is being created.
	WriteBufferSize int

	// WriteIntervals sets the duration that takes the connection to
	// automatically flush the buffers. If set to zero, buffering
	// will be disabled.
	WriteInterval time.Duration
}

func (dc *ConnConfig) Dial(network string, laddr *net.UDPAddr, raddr *net.UDPAddr) (types.PacketConn, error) {
	// check inputs
	switch network {
	case "udp", "udp4", "udp6":
	default:
		return nil, types.ErrUnknownNetwork
	}
	if raddr == nil {
		return nil, types.ErrMissingAddr
	}
	conn, err := net.DialUDP(network, laddr, raddr)
	if err != nil {
		return nil, err
	}
	if dc.ReadBufferSize > 0 {
		_ = conn.SetReadBuffer(dc.ReadBufferSize)
	}
	if dc.WriteBufferSize > 0 {
		_ = conn.SetWriteBuffer(dc.WriteBufferSize)
	}
	rawConn, err := raw.WrapConn(conn, dc.WriteInterval)
	if err != nil {
		return nil, err
	}
	return WrapConnectedConn(rawConn, dc)
}

func Dial(network string, laddr *net.UDPAddr, raddr *net.UDPAddr) (types.PacketConn, error) {
	return (&ConnConfig{}).Dial(network, laddr, raddr)
}

type bufferedConn interface {
	SetReadBuffer(bufferSize int) error
	SetWriteBuffer(bufferSize int) error
}

func wrapDialed(rawConn types.RawConn, dc *ConnConfig) (*supConn, error) {
	if dc == nil {
		dc = &ConnConfig{}
	}
	if bConn, ok := rawConn.(bufferedConn); ok {
		if dc.ReadBufferSize > 0 {
			_ = bConn.SetReadBuffer(dc.ReadBufferSize)
		}
		if dc.WriteBufferSize > 0 {
			_ = bConn.SetWriteBuffer(dc.WriteBufferSize)
		}
	}

	return wrapConn(rawConn, false, nil, 1)
}

func WrapConnectedConn(rawConn types.RawConn, dc *ConnConfig) (types.PacketConn, error) {
	if dc == nil {
		dc = &ConnConfig{}
	}
	if bConn, ok := rawConn.(bufferedConn); ok {
		if dc.ReadBufferSize > 0 {
			_ = bConn.SetReadBuffer(dc.ReadBufferSize)
		}
		if dc.WriteBufferSize > 0 {
			_ = bConn.SetWriteBuffer(dc.WriteBufferSize)
		}
	}

	return &ConnectedConn{base: rawConn}, nil
}

func WrapUnconnectedConn(rawConn types.RawConn, dc *ConnConfig) (types.SuperConn, types.GetSubConnFunc, error) {
	if dc == nil {
		dc = &ConnConfig{}
	}
	super, err := wrapDialed(rawConn, dc)
	if err != nil {
		return nil, nil, err
	}
	return super, func(raddr net.Addr) (c types.PacketConn, err error) {
		c, _, err = super.getSubConn(raddr, nil, false)
		return
	}, nil
}
