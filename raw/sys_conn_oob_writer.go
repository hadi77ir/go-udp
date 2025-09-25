//go:build darwin || linux || freebsd

package raw

import (
	"github.com/valyala/bytebufferpool"
	"golang.org/x/net/ipv4"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type oobWriter interface {
	io.Closer
	Write(b []byte, oob []byte, addr net.Addr) (dataN int, oobN int, err error)
}

type batchWriter struct {
	writer      sysBatchWriter
	isConnected bool

	writePos  uint8
	lastWrite time.Time

	// Packets put in the connection queue, but not yet sent to the kernel.
	messages []ipv4.Message
	buffers  []*bytebufferpool.ByteBuffer

	oobBuffers []*bytebufferpool.ByteBuffer
	mutex      sync.Mutex
	closed     atomic.Bool

	buffersReleased bool
	onClose         func()
}

func (w *batchWriter) Write(b []byte, oob []byte, addr net.Addr) (dataN int, oobN int, err error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.closed.Load() {
		return 0, 0, ErrClosed
	}

	// put message into queue
	msg := w.messages[w.writePos]
	dataN = copy(msg.Buffers[0], b)
	oobN = copy(msg.OOB, oob)
	msg.Buffers[0] = msg.Buffers[0][:dataN]
	msg.OOB = msg.OOB[oobN:]
	// if socket is a "connected socket", leave the addr empty (nil).
	if !w.isConnected {
		msg.Addr = addr
	}
	w.writePos++
	if w.writePos > batchSize { // queue is full. Flush this batch of messages.
		_, err := w.flush()
		if err != nil {
			return 0, 0, err
		}
	}
	return
}

func (w *batchWriter) flush() (int, error) {
	w.messages = w.messages[:batchSize]

	w.lastWrite = time.Now()
	w.writePos = 0

	txIdx := 0
	for txIdx < len(w.messages) {
		n, err := w.writer.WriteBatch(w.messages[txIdx:w.writePos], 0)
		if n == 0 || err != nil {
			return 0, err
		}
		txIdx += n
	}

	for i := uint8(0); i < w.writePos; i++ {
		// renew buffers
		renewBuffer(w.buffers, bufferPool, int(i), MaxPacketBufferSize)
		renewBuffer(w.oobBuffers, oobBufferPool, int(i), oobBufferSize)

		// set new slices
		w.messages[i].Buffers[0] = w.buffers[i].B
		w.messages[i].OOB = w.buffers[i].B
	}
	return txIdx, nil
}

func (w *batchWriter) Close() error {
	w.closed.Store(true)
	w.mutex.Lock()
	if w.writePos > 0 {
		_, _ = w.flush()
	}
	// release all buffers
	if !w.buffersReleased {
		for i := 0; i < batchSize; i++ {
			bufferPool.Put(w.buffers[i])
			oobBufferPool.Put(w.oobBuffers[i])
		}
		w.buffersReleased = true
	}
	w.mutex.Unlock()

	err := w.writer.Close()
	if w.onClose != nil {
		w.onClose()
	}
	return err
}

func newBatchWriter(bw sysBatchWriter, isConnected bool, interval time.Duration, onClose func()) oobWriter {
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

	writer := &batchWriter{
		writer:      bw,
		isConnected: isConnected,
		buffers:     bodyBuffers,
		oobBuffers:  oobBuffers,
		messages:    msgs,
		writePos:    batchSize,
		onClose:     onClose,
	}

	// interval flush worker
	go func() {
		writeTicker := time.NewTicker(interval / 2)
		defer writeTicker.Stop()
		for !writer.closed.Load() {
			<-writeTicker.C
			writer.mutex.Lock()
			if writer.writePos > 0 && time.Since(writer.lastWrite) >= interval {
				_, _ = writer.flush()
			}
			writer.mutex.Unlock()
		}
	}()

	return writer
}

var _ oobWriter = &batchWriter{}

// implement basic writer
type basicWriter struct {
	OOBCapablePacketConn
	isConnected bool
}

func (w *basicWriter) Write(b []byte, oob []byte, addr net.Addr) (dataN int, oobN int, err error) {
	if w.isConnected {
		if cc, ok := w.OOBCapablePacketConn.(connectedConn); ok {
			dataN, err = cc.Write(b)
			return
		} else {
			addr = nil
		}
	}
	return w.OOBCapablePacketConn.WriteMsgUDP(b, oob, addr.(*net.UDPAddr))
}

var _ oobWriter = &basicWriter{}
