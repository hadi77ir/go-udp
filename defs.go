package udp

import (
	"github.com/hadi77ir/go-udp/types"
	"github.com/valyala/bytebufferpool"
)

const (
	receiveMTU           = 8192
	sendMTU              = 1500
	oobSize              = 128
	defaultListenBacklog = 128 // same as Linux default
	receiveBacklog       = 1024
)

// receivedPacket represents a single packet received by batch conn
// which would be put to
type receivedPacket struct {
	data *bytebufferpool.ByteBuffer
	oob  *bytebufferpool.ByteBuffer
	ecn  types.ECN
}
