//go:build !darwin && !linux && !freebsd && !windows

package raw

import (
	"net"
)

func newConn(c net.PacketConn, supportsDF bool) (*BasicConn, error) {
	return &BasicConn{PacketConn: c, supportsDF: supportsDF}, nil
}

func InspectReadBuffer(any) (int, error)  { return 0, nil }
func InspectWriteBuffer(any) (int, error) { return 0, nil }

type packetInfo struct {
	addr net.IP
}

func (i *packetInfo) OOB() []byte { return nil }
