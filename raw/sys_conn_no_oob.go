//go:build !darwin && !linux && !freebsd && !windows

package raw

import (
	"net"
)

func newConn(c net.PacketConn, supportsDF bool) (*BasicConn, error) {
	// Check if this socket is a "connected" socket.
	isConnected := false
	if ra, ok := c.(remoteAddrSock); ok && ra.RemoteAddr() != nil {
		// this is a connected socket. use nil remote address.
		isConnected = true
	}
	return &BasicConn{PacketConn: c, supportsDF: supportsDF, isConnected: isConnected}, nil
}

func InspectReadBuffer(any) (int, error)  { return 0, nil }
func InspectWriteBuffer(any) (int, error) { return 0, nil }

type packetInfo struct {
	addr net.IP
}

func (i *packetInfo) OOB() []byte { return nil }
