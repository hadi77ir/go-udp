//go:build darwin || linux || freebsd

package raw

import (
	"github.com/valyala/bytebufferpool"
	"golang.org/x/net/ipv4"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

var bufferPool = &bytebufferpool.Pool{}
var oobBufferPool = &bytebufferpool.Pool{}

type oobReader interface {
	io.Closer
	Read(b []byte, oob []byte) (dataN int, oobN int, addr net.Addr, err error)
}

type batchReader struct {
	reader  sysBatchReader
	readPos uint8
	// Packets received from the kernel, but not yet returned by ReadPacket().
	messages   []ipv4.Message
	buffers    []*bytebufferpool.ByteBuffer
	oobBuffers []*bytebufferpool.ByteBuffer

	mutex           sync.Mutex
	closed          atomic.Bool
	buffersReleased bool
	onClose         func()
}

func (r *batchReader) Read(b []byte, oob []byte) (bytesRead int, oobRead int, remoteAddr net.Addr, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.closed.Load() {
		return 0, 0, nil, ErrClosed
	}

	if len(r.messages) == int(r.readPos) { // all messages read. Read the next batch of messages.
		r.messages = r.messages[:batchSize]
		// replace buffers data buffers up to the packet that has been consumed during the last ReadBatch call
		for i := uint8(0); i < r.readPos; i++ {
			// renew buffers
			renewBuffer(r.buffers, bufferPool, int(i), MaxPacketBufferSize)
			renewBuffer(r.oobBuffers, oobBufferPool, int(i), oobBufferSize)

			// set new slices
			r.messages[i].Buffers[0] = r.buffers[i].B
			r.messages[i].OOB = r.oobBuffers[i].B
		}
		r.readPos = 0

		n, err := r.reader.ReadBatch(r.messages, 0)
		if n == 0 || err != nil {
			return 0, 0, nil, err
		}
		r.messages = r.messages[:n]
	}

	msg := r.messages[r.readPos]
	r.readPos++

	// set returning data
	// IP is allocated by runtime, so no need to copy
	remoteAddr = msg.Addr
	bytesRead = copy(b, msg.Buffers[0][:msg.N])
	oobData := msg.OOB[:msg.NN]
	oobRead = copy(oob, oobData)

	return bytesRead, oobRead, remoteAddr, nil
}

func (r *batchReader) Close() error {
	r.closed.Store(true)
	r.mutex.Lock()
	// release all buffers
	if !r.buffersReleased {
		for i := 0; i < batchSize; i++ {
			bufferPool.Put(r.buffers[i])
			oobBufferPool.Put(r.oobBuffers[i])
		}
		r.buffersReleased = true
	}
	r.mutex.Unlock()

	err := r.reader.Close()
	if r.onClose != nil {
		r.onClose()
	}
	return err
}

func newBatchReader(br sysBatchReader, onClose func()) oobReader {
	bodyBuffers := make([]*bytebufferpool.ByteBuffer, batchSize)
	oobBuffers := make([]*bytebufferpool.ByteBuffer, batchSize)
	msgs := make([]ipv4.Message, batchSize)
	for i := int(0); i < batchSize; i++ {
		// preallocate msg body buffers
		bodyBuffers[i] = bufferPool.Get()
		if bodyBuffers[i].Len() < MaxPacketBufferSize {
			bodyBuffers[i].B = make([]byte, MaxPacketBufferSize)
		}
		// preallocate oob buffers
		oobBuffers[i] = oobBufferPool.Get()
		if oobBuffers[i].Len() < MaxPacketBufferSize {
			oobBuffers[i].B = make([]byte, MaxPacketBufferSize)
		}

		if msgs[i].Buffers == nil {
			msgs[i].Buffers = make([][]byte, 1)
		}
		msgs[i].Buffers[0] = oobBuffers[i].B
		msgs[i].OOB = oobBuffers[i].B
	}

	return &batchReader{
		reader:     br,
		buffers:    bodyBuffers,
		oobBuffers: oobBuffers,
		messages:   msgs,
		readPos:    batchSize,
		onClose:    onClose,
	}
}

var _ oobReader = &batchReader{}

func renewBuffer(buffers []*bytebufferpool.ByteBuffer, pool *bytebufferpool.Pool, idx int, size int) {
	if len(buffers[idx].B) < size {
		pool.Put(buffers[idx])
		buffers[idx] = pool.Get()
		// if len is less than max, use new
		if len(buffers[idx].B) < size {
			buffers[idx].B = make([]byte, size)
		}
	}
}
