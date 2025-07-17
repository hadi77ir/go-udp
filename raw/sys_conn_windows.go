//go:build windows

package raw

import (
	"net/netip"
	"syscall"

	"golang.org/x/sys/windows"
)

func newConn(c OOBCapablePacketConn, supportsDF bool) (*BasicConn, error) {
	// Check if this socket is a "connected" socket.
	isConnected := false
	if ra, ok := c.(remoteAddrSock); ok && ra.RemoteAddr() != nil {
		// this is a connected socket. use nil remote address.
		isConnected = true
	}
	return &BasicConn{PacketConn: c, supportsDF: supportsDF, isConnected: isConnected}, nil
}

func InspectReadBuffer(c syscall.RawConn) (int, error) {
	var size int
	var serr error
	if err := c.Control(func(fd uintptr) {
		size, serr = windows.GetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_RCVBUF)
	}); err != nil {
		return 0, err
	}
	return size, serr
}

func InspectWriteBuffer(c syscall.RawConn) (int, error) {
	var size int
	var serr error
	if err := c.Control(func(fd uintptr) {
		size, serr = windows.GetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_SNDBUF)
	}); err != nil {
		return 0, err
	}
	return size, serr
}

type packetInfo struct {
	addr netip.Addr
}

func (i *packetInfo) OOB() []byte { return nil }
