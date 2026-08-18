//go:build linux
// +build linux

package transport

import (
	gerrors "errors"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// stdioReconnectInterval is the fixed cadence for re-dialing a dropped connection.
const stdioReconnectInterval = 100 * time.Millisecond

// stdioReplayBytes is how many of the most-recently-written bytes we keep around
// so we can re-send them after reconnecting. When the host connection drops, any
// bytes that write() already accepted but the host never read are gone. Re-sending
// this tail recovers them (at the cost of re-sending some the host did read).
//
// So it has to be at least as large as the most data that can be "in flight" -
// buffered somewhere between our write() and the host actually reading it - when a
// drop happens. The important case is the live-migration blackout: the guest VM is
// paused and the host tears down its side of the socket, discarding whatever it had
// buffered. That in-flight data sits in two fixed-size OS buffers:
//
//   - Guest send buffer (guest -> host): 24 KiB. Fixed by the Linux hv_sock driver
//     (RINGBUFFER_HVS_SND_SIZE); we never enlarge it with SO_SNDBUF.
//   - Host receive buffer: 64 KiB. The Windows hvsocket default (SO_RCVBUF).
//
// So the most we can lose in one drop is about 24 + 64 = ~88 KiB.
// 128 KiB rounds that up with headroom. These two sizes are set by the OS and
// do not grow with how long the container runs or how fast it logs,
// so a bigger buffer wouldn't help - and a smaller one could actually drop
// logs if the host falls behind and both buffers are full when the connection drops.
const stdioReplayBytes = 128 * 1024 // 128 KiB

// reconnectConnection is a Connection that transparently re-dials the same port
// when a write fails because the host end of an established connection went away.
type reconnectConnection struct {
	t       Transport
	port    uint32
	stopped *atomic.Bool

	mu     sync.Mutex
	conn   Connection
	replay *replayBuffer
}

var _ Connection = &reconnectConnection{}

// NewReconnectConnection wraps a live host connection so a container's outbound
// stream keeps flowing across a brief host disappearance by re-dialing and resuming.
func NewReconnectConnection(t Transport, port uint32, conn Connection, stopped *atomic.Bool) Connection {
	return &reconnectConnection{t: t, port: port, conn: conn, stopped: stopped, replay: newReplayBuffer(stdioReplayBytes)}
}

// current returns the connection in use, waiting out any in-progress re-dial so it
// never hands back a broken one.
func (c *reconnectConnection) current() Connection {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.conn
}

// Write sends bytes to the host, transparently reconnecting and resuming on a
// mid-stream drop; a caller only sees an error once reconnection is abandoned at teardown.
func (c *reconnectConnection) Write(b []byte) (int, error) {
	// Retain each chunk we successfully write so redial can re-send it if the
	// drop lost it (see stdioReplayBytes). written tracks progress through b.
	written := 0
	for {
		n, err := c.current().Write(b[written:])
		if n > 0 {
			c.replay.append(b[written : written+n])
			written += n
		}
		if err == nil {
			return written, nil
		}
		if !isDisconnectErr(err) {
			return written, err
		}
		if rerr := c.redial(); rerr != nil {
			return written, err
		}
	}
}

// redial closes the dead connection and re-dials the same host endpoint every 100ms
// until reconnected or teardown is signalled, holding the lock so current never sees the broken conn.
func (c *reconnectConnection) redial() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		_ = c.conn.Close()
	}
	for {
		if c.stopped.Load() {
			return gerrors.New("stdio reconnect stopped")
		}
		conn, err := c.t.Dial(c.port)
		if err != nil {
			time.Sleep(stdioReconnectInterval)
			continue
		}
		// Re-send the retained tail to recover bytes the dropped connection lost.
		// This may re-deliver some the host already read, which is fine for logs.
		if tail := c.replay.bytes(); len(tail) > 0 {
			if _, werr := conn.Write(tail); werr != nil {
				_ = conn.Close()
				time.Sleep(stdioReconnectInterval)
				continue
			}
		}
		c.conn = conn
		logrus.WithField("port", c.port).Info("opengcs::reconnectConnection - reconnected stdio")
		return nil
	}
}

// Read reads from the current host connection; used only by the relay's clean-close handshake.
func (c *reconnectConnection) Read(b []byte) (int, error) { return c.current().Read(b) }

// Close tears down the current host connection.
func (c *reconnectConnection) Close() error { return c.current().Close() }

// CloseRead closes the read half of the current host connection.
func (c *reconnectConnection) CloseRead() error { return c.current().CloseRead() }

// CloseWrite closes the write half, signalling end-of-stream to the host.
func (c *reconnectConnection) CloseWrite() error { return c.current().CloseWrite() }

// File exposes the current connection's file; a later reconnect is not reflected in
// an fd already handed out, so this is only meaningful before any drop.
func (c *reconnectConnection) File() (*os.File, error) { return c.current().File() }

// isDisconnectErr reports whether a write failure means the host end went away
// (worth reconnecting) rather than an unrecoverable error to surface to the caller.
func isDisconnectErr(err error) bool {
	var errno syscall.Errno
	if gerrors.As(err, &errno) {
		switch errno {
		case syscall.EPIPE, syscall.ECONNRESET, syscall.ENOTCONN:
			return true
		}
	}
	return false
}

// replayBuffer keeps the most recent max bytes written to the connection so they
// can be re-sent after a reconnect; older bytes are dropped.
//
// To avoid recopying on every append, it lets the backing slice grow to 2*max and
// only then trims it back to the last max bytes. A trim (an O(max) copy) therefore
// happens at most once per max bytes appended, so on average an append costs time
// proportional to the bytes added, and the buffer never holds more than ~2*max.
type replayBuffer struct {
	data []byte
	max  int
}

func newReplayBuffer(max int) *replayBuffer {
	return &replayBuffer{max: max}
}

// append records p, keeping only the most recent max bytes.
func (r *replayBuffer) append(p []byte) {
	r.data = append(r.data, p...)
	// Trim only once the buffer exceeds 2*max, so the copy happens ~once per max bytes.
	if len(r.data) > 2*r.max {
		// Copy the last max bytes to the front, reusing the array (no realloc, no overlap).
		r.data = append(r.data[:0], r.data[len(r.data)-r.max:]...)
	}
}

// bytes returns a copy of the retained tail (at most max bytes), in write order.
func (r *replayBuffer) bytes() []byte {
	tail := r.data
	if len(tail) > r.max {
		tail = tail[len(tail)-r.max:]
	}
	out := make([]byte, len(tail))
	copy(out, tail)
	return out
}
