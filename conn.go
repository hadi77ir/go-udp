package udp

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"github.com/hadi77ir/go-ringqueue"
	"github.com/pion/transport/v3/deadline"

	"github.com/hadi77ir/go-udp/types"
)

// Conn augments a connection-oriented connection over a UDP PacketConn.
type Conn struct {
	super *supConn

	rAddr net.Addr

	queue ringqueue.RingQueue[receivedPacket]

	doneCh   chan struct{}
	doneOnce sync.Once

	writeDeadline *deadline.Deadline

	ecnDefault types.ECN
	cap        types.ConnCapabilities
}

// Read reads from c into p.
func (c *Conn) Read(p []byte, oob []byte) (int, int, types.ECN, error) {
	pkt, _, err := c.queue.Pop()
	if err != nil {
		select {
		case <-c.doneCh:
			return 0, 0, c.ecnDefault, io.EOF
		default:
			// todo: convert errors
			return 0, 0, c.ecnDefault, err
		}
	}
	n := copy(p, pkt.data.B)
	oobN := copy(oob, pkt.oob.B)
	// release buffers
	c.super.bufPool.Put(pkt.data)
	c.super.oobPool.Put(pkt.oob)
	return n, oobN, pkt.ecn, nil
}

// write deadline only casually checked
func (c *Conn) checkWriteDeadline() error {
	select {
	case <-c.doneCh:
		return types.ErrClosedSuperConn
	case <-c.writeDeadline.Done():
		return context.DeadlineExceeded
	default:
	}
	return nil
}

// Write writes len(p) bytes from p to the DTLS connection.
func (c *Conn) Write(p []byte, oob []byte, gsoSize int, ecn types.ECN) (n int, oobN int, err error) {
	// write deadline only casually checked as we will be writing to a buffer, and it won't be blocking too much
	if err := c.checkWriteDeadline(); err != nil {
		return 0, 0, err
	}

	return c.super.pConn.WritePacket(p, oob, uint16(gsoSize), ecn, c.rAddr)
}

// Close closes the conn and releases any Read calls.
func (c *Conn) Close() error {
	var err error
	c.doneOnce.Do(func() {
		c.super.connWG.Done()
		close(c.doneCh)
		c.super.connLock.Lock()
		delete(c.super.conns, c.rAddr.String())
		nConns := len(c.super.conns)
		c.super.connLock.Unlock()

		if isAccepting, ok := c.super.accepting.Load().(bool); nConns == 0 && !isAccepting && ok {
			// Wait if this is the final connection
			c.super.readWG.Wait()
			if errClose, ok := c.super.errClose.Load().(error); ok {
				err = errClose
			}
		} else {
			err = nil
		}

		if errBuf := c.queue.Close(); errBuf != nil && err == nil {
			err = errBuf
		}
	})

	return err
}

// LocalAddr implements net.Conn.LocalAddr.
func (c *Conn) LocalAddr() net.Addr {
	return c.super.pConn.LocalAddr()
}

// RemoteAddr implements net.Conn.RemoteAddr.
func (c *Conn) RemoteAddr() net.Addr {
	return c.rAddr
}

// SetDeadline implements net.Conn.SetDeadline.
func (c *Conn) SetDeadline(t time.Time) error {
	_ = c.SetWriteDeadline(t)

	return c.SetReadDeadline(t)
}

// SetReadDeadline implements net.Conn.SetDeadline.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.queue.SetPopDeadline(t)
}

// SetWriteDeadline implements net.Conn.SetDeadline.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline.Set(t)
	return nil
}

func (c *Conn) Capabilities() types.ConnCapabilities {
	return c.super.pConnCaps
}

var _ types.PacketConn = &Conn{}
