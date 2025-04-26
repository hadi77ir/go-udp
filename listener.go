// Package conn provides a connection-oriented listener over a UDP PacketConn
package udp

import (
	"github.com/hadi77ir/go-udp/raw"
	"github.com/hadi77ir/go-udp/types"
	"net"
)

// ListenConfig stores options for listening to an address.
type ListenConfig struct {
	// Backlog defines the maximum length of the queue of pending
	// connections. It is equivalent of the backlog argument of
	// POSIX listen function.
	// If a connection request arrives when the queue is full,
	// the request will be silently discarded, unlike TCP.
	// Set zero to use default value 128 which is same as Linux default.
	Backlog int

	// AcceptFilter determines whether the new conn should be made for
	// the incoming packet. If not set, any packet creates new conn.
	AcceptFilter func([]byte) bool

	ConnConfig
}

// Listen creates a new listener based on the ListenConfig.
func (lc *ListenConfig) Listen(network string, laddr *net.UDPAddr) (types.SuperConn, error) {
	if lc.Backlog == 0 {
		lc.Backlog = defaultListenBacklog
	}

	conn, err := net.ListenUDP(network, laddr)
	if err != nil {
		return nil, err
	}

	rawConn, err := raw.WrapConn(conn, lc.WriteInterval)
	//rawConn := &udp.BasicConn{PacketConn: conn}
	if err != nil {
		return nil, err
	}

	return wrapConn(rawConn, true, lc.AcceptFilter, lc.Backlog)
}

// Listen creates a new listener using default ListenConfig.
func Listen(network string, laddr *net.UDPAddr) (types.SuperConn, error) {
	return (&ListenConfig{}).Listen(network, laddr)
}

func WrapListenConn(rawConn types.RawConn, lc *ListenConfig) (types.SuperConn, error) {
	return wrapConn(rawConn, true, lc.AcceptFilter, lc.Backlog)
}
