package udp

import (
	"net"
	"sync"
	"time"

	"github.com/hadi77ir/go-udp/raw"
	"github.com/hadi77ir/go-udp/types"
)

// dialed connections have no difference with listeners

// dialSuperConn is a map of laddr to listener
var dialSuperConns map[string]*supConn = make(map[string]*supConn)
var dialSuperConnsMutex *sync.Mutex = &sync.Mutex{}

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

func (dc *ConnConfig) Dial(network string, laddr *net.UDPAddr, raddr *net.UDPAddr) (*Conn, error) {
	// check inputs
	switch network {
	case "udp", "udp4", "udp6":
	default:
		return nil, types.ErrUnknownNetwork
	}
	if raddr == nil {
		return nil, types.ErrMissingAddr
	}

	super, err := getSuperConn(network, laddr, raddr, dc)
	if err != nil {
		return nil, err
	}

	// - get new conn from listener for raddr
	sub, isNew, err := super.getSubConn(raddr, nil, false)
	if err != nil {
		return nil, err
	}
	if sub != nil {
		// we don't want to mess with already assigned connections.
		if !isNew {
			return nil, types.ErrAlreadyInUse
		}
		return sub, nil
	}
	return nil, types.ErrUnexpectedNil
}

func (dc *ConnConfig) DialSuper(network string, laddr *net.UDPAddr, raddr *net.UDPAddr) (types.SuperConn, error) {
	return getSuperConn(network, laddr, raddr, dc)
}
func Dial(network string, laddr *net.UDPAddr, raddr *net.UDPAddr) (*Conn, error) {
	dc := &ConnConfig{}
	return dc.Dial(network, laddr, raddr)
}

func DialSuper(network string, laddr *net.UDPAddr, raddr *net.UDPAddr) (types.SuperConn, error) {
	dc := &ConnConfig{}
	return dc.DialSuper(network, laddr, raddr)
}

func getSuperConn(network string, laddr *net.UDPAddr, raddr *net.UDPAddr, dc *ConnConfig) (*supConn, error) {
	dialSuperConnsMutex.Lock()
	defer dialSuperConnsMutex.Unlock()
	// - get a superConn
	// -- check if dialSuperConn have an entry for laddr
	super, ok := dialSuperConns[laddr.String()]
	// -- if not, then create a new listener
	if !ok {
		var err error
		super, err = superDialNew(network, laddr, raddr, dc)
		if err != nil {
			return nil, err
		}
		dialSuperConns[laddr.String()] = super
	}
	return super, nil
}

func superDialNew(network string, laddr *net.UDPAddr, raddr *net.UDPAddr, dc *ConnConfig) (*supConn, error) {
	conn, err := net.DialUDP(network, laddr, raddr)
	if err != nil {
		return nil, err
	}

	rawConn, err := raw.WrapConn(conn, dc.WriteInterval)
	//rawConn := &udp.BasicConn{PacketConn: conn}
	if err != nil {
		return nil, err
	}
	return wrapDialed(rawConn, dc)
}

type bufferedConn interface {
	SetReadBuffer(bufferSize int) error
	SetWriteBuffer(bufferSize int) error
}

func wrapDialed(rawConn types.RawConn, dc *ConnConfig) (*supConn, error) {
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

func WrapDialedConn(rawConn types.RawConn, dc *ConnConfig) (types.SuperConn, types.GetSubConnFunc, error) {
	super, err := wrapDialed(rawConn, dc)
	if err != nil {
		return nil, nil, err
	}
	return super, func(raddr net.Addr) (c types.PacketConn, err error) {
		c, _, err = super.getSubConn(raddr, nil, false)
		return
	}, nil
}
