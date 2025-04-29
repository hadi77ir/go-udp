package types

import (
	"io"
	"net"
	"time"
)

type ConnCapabilities int

const (
	// ConnCapabilityDontFragment means this connection has the Don't Fragment (DF) bit set.
	// This means it makes to run DPLPMTUD.
	ConnCapabilityDontFragment = ConnCapabilities(1 << iota)
	// ConnCapabilityGSO means GSO (Generic Segmentation Offload) supported
	ConnCapabilityGSO
	// ConnCapabilityECN means ECN (Explicit Congestion Notifications) supported
	ConnCapabilityECN
)

func (c ConnCapabilities) DF() bool {
	return c&ConnCapabilityDontFragment != 0
}

func (c ConnCapabilities) GSO() bool {
	return c&ConnCapabilityGSO != 0
}

func (c ConnCapabilities) ECN() bool {
	return c&ConnCapabilityECN != 0
}

func NewConnCapabilities(df, gso, ecn bool) ConnCapabilities {
	val := ConnCapabilities(0)
	if df {
		val |= ConnCapabilityDontFragment
	}
	if gso {
		val |= ConnCapabilityGSO
	}
	if ecn {
		val |= ConnCapabilityECN
	}
	return val
}

// RawConn is a connection that allow reading of a receivedPackeh.
type RawConn interface {
	ReadPacket(b []byte, oob []byte) (bytesRead int, oobRead int, ecn ECN, remoteAddr net.Addr, err error)
	// WritePacket writes a packet on the wire.
	// gsoSize is the size of a single packet, or 0 to disable GSO.
	// It is invalid to set gsoSize if capabilities.GSO is not set.
	WritePacket(b []byte, oob []byte, gsoSize uint16, ecn ECN, addr net.Addr) (bytesWritten int, oobWritten int, err error)
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetReadDeadline(time.Time) error
	io.Closer

	Capabilities() ConnCapabilities
}
