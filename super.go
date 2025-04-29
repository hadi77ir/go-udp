package udp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hadi77ir/go-ringqueue"
	"github.com/pion/transport/v3/deadline"
	"github.com/valyala/bytebufferpool"

	"github.com/hadi77ir/go-udp/types"
)

// supConn augments a connection-oriented SuperConn over a UDP PacketConn.
type supConn struct {
	pConn     types.RawConn
	pConnCaps types.ConnCapabilities

	readBatchSize int

	accepting    atomic.Value // bool
	acceptCh     chan *Conn
	doneCh       chan struct{}
	doneOnce     sync.Once
	acceptFilter func([]byte) bool

	connLock sync.Mutex
	conns    map[string]*Conn
	connWG   *sync.WaitGroup

	readWG   sync.WaitGroup
	errClose atomic.Value // error

	readDoneCh chan struct{}
	errRead    atomic.Value // error

	deadline *deadline.Deadline

	bufPool *bytebufferpool.Pool
	oobPool *bytebufferpool.Pool

	isListener bool
}

var _ types.SuperConn = &supConn{}

func wrapConn(rawConn types.RawConn, isListener bool, acceptFilter func([]byte) bool, backlog int) (*supConn, error) {

	listener := &supConn{
		pConn:        rawConn,
		pConnCaps:    rawConn.Capabilities(),
		acceptCh:     make(chan *Conn, backlog),
		conns:        make(map[string]*Conn),
		doneCh:       make(chan struct{}),
		acceptFilter: acceptFilter,
		connWG:       &sync.WaitGroup{},
		readDoneCh:   make(chan struct{}),
		bufPool:      &bytebufferpool.Pool{},
		oobPool:      &bytebufferpool.Pool{},
		deadline:     deadline.New(),
		isListener:   isListener,
	}

	listener.accepting.Store(true)
	listener.connWG.Add(1)
	listener.readWG.Add(2) // wait readLoop and Close execution routine

	go listener.readLoop()
	go func() {
		listener.connWG.Wait()
		if err := listener.pConn.Close(); err != nil {
			listener.errClose.Store(err)
		}
		listener.readWG.Done()
	}()

	return listener, nil
}

// Close closes the listener.
// Any blocked Accept operations will be unblocked and return errors.
func (l *supConn) Close() error {
	var err error
	l.doneOnce.Do(func() {
		l.accepting.Store(false)
		close(l.doneCh)

		l.connLock.Lock()
		// Close unaccepted connections
	lclose:
		for {
			select {
			case c := <-l.acceptCh:
				close(c.doneCh)
				delete(l.conns, c.rAddr.String())

			default:
				break lclose
			}
		}
		nConns := len(l.conns)
		l.connLock.Unlock()

		l.connWG.Done()

		if nConns == 0 {
			// Wait if this is the final connection
			l.readWG.Wait()
			if errClose, ok := l.errClose.Load().(error); ok {
				err = errClose
			}
		} else {
			err = nil
		}
	})

	return err
}

func (l *supConn) Capabilities() types.ConnCapabilities {
	return l.pConnCaps
}

// Addr returns the listener's network address.
func (l *supConn) Addr() net.Addr {
	return l.pConn.LocalAddr()
}

func (l *supConn) SetDeadline(t time.Time) error {
	l.deadline.Set(t)
	return nil
}

// Accept waits for and returns the next connection to the listener.
func (l *supConn) Accept() (types.PacketConn, error) {
	if !l.isListener {
		return nil, errors.ErrUnsupported
	}
	select {
	case c := <-l.acceptCh:
		l.connWG.Add(1)

		return c, nil

	case <-l.readDoneCh:
		err, _ := l.errRead.Load().(error)

		// todo: convert errors
		return nil, err
	case <-l.deadline.Done():
		return nil, context.DeadlineExceeded
	case <-l.doneCh:
		return nil, types.ErrClosedSuperConn
	}
}

// readLoop has two tasks:
//  1. Dispatching incoming packets to the correct Conn.
//     It can therefore not be ended until all Conns are closed.
//  2. Creating a new Conn when receiving from a new remote.
func (l *supConn) readLoop() {
	defer l.readWG.Done()
	defer close(l.readDoneCh)
	for {
		buf := l.bufPool.Get()
		if len(buf.B) < receiveMTU {
			buf.B = make([]byte, receiveMTU)
		}
		oobBuf := l.oobPool.Get()
		if len(oobBuf.B) < oobSize {
			oobBuf.B = make([]byte, oobSize)
		}

		n, oobN, ecn, raddr, err := l.pConn.ReadPacket(buf.B, oobBuf.B)
		if err != nil {
			l.errRead.Store(err)

			return
		}
		buf.B = buf.B[:n]
		oobBuf.B = oobBuf.B[:oobN]
		l.dispatchMsg(raddr, buf, oobBuf, ecn)
	}
}

func (l *supConn) dispatchMsg(addr net.Addr, buf *bytebufferpool.ByteBuffer, oobBuf *bytebufferpool.ByteBuffer, ecn types.ECN) {
	conn, _, _ := l.getSubConn(addr, buf.B, true)
	if conn != nil {
		// put buffers in connection ring
		_, _ = conn.queue.Push(receivedPacket{data: buf, oob: oobBuf, ecn: ecn})
	}
}

func (l *supConn) getSubConn(rAddr net.Addr, buf []byte, filter bool) (c *Conn, isNew bool, err error) {
	l.connLock.Lock()
	defer l.connLock.Unlock()
	conn, ok := l.conns[l.rAddrString(rAddr)]
	if ok {
		return conn, false, nil
	}
	if isAccepting, ok := l.accepting.Load().(bool); !isAccepting || !ok {
		return nil, false, types.ErrClosedSuperConn
	}
	// NOTE: don't use buf if filter is false. buf might be nil.
	if filter && l.acceptFilter != nil {
		if !l.acceptFilter(buf) {
			return nil, false, nil
		}
	}
	conn = l.newSubConn(rAddr)
	if l.isListener {
		select {
		case l.acceptCh <- conn:
			l.conns[l.rAddrString(rAddr)] = conn
		default:
			return nil, false, types.ErrListenQueueExceeded
		}
	}
	return conn, true, nil
}

func (l *supConn) rAddrString(addr net.Addr) string {
	if addr == nil {
		addr = l.pConn.RemoteAddr()
		if addr == nil {
			return "<nil>"
		}
	}
	return addr.String()
}

func (l *supConn) newSubConn(rAddr net.Addr) *Conn {
	queue, err := ringqueue.NewSafe[receivedPacket](receiveBacklog, ringqueue.WhenFullError, ringqueue.WhenEmptyBlock, l.onQueueClose)
	if err != nil {
		panic(fmt.Sprintf("queue is not working: %s ", err))
	}
	ecnDefault := types.ECNUnsupported
	if l.pConnCaps.ECN() {
		ecnDefault = types.ECNNon
	}
	return &Conn{
		super:         l,
		rAddr:         rAddr,
		queue:         queue,
		doneCh:        make(chan struct{}),
		writeDeadline: deadline.New(),
		ecnDefault:    ecnDefault,
	}
}

func (l *supConn) onQueueClose(data receivedPacket) {
	l.putBuffers(data.data, data.oob)
	return
}
func (l *supConn) putBuffers(data *bytebufferpool.ByteBuffer, oob *bytebufferpool.ByteBuffer) {
	if data != nil {
		l.bufPool.Put(data)
	}
	if oob != nil {
		l.oobPool.Put(oob)
	}
}
