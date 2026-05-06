//go:build linux
// +build linux

package stdio

import (
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/internal/guest/transport"
	"github.com/sirupsen/logrus"
)

// runRedial pacing. Tight fixed interval matches the bridge reconnect loop
// in cmd/gcs/main.go; the destination listener should be live as soon as
// the bridge re-accepts. maxRedialAttempts bounds the goroutine's lifetime
// against a permanently broken peer.
const (
	redialInterval    = 100 * time.Millisecond
	maxRedialAttempts = 60
)

// ConnSlot wraps a transport.Connection so the underlying connection can be
// replaced at runtime. While disconnected, Read and Write block (parking
// the relay in acquire) so the producing process back-pressures on its own
// kernel pipe instead of losing bytes. Set installs a fresh connection and
// wakes blocked relays.
type ConnSlot struct {
	mu     sync.Mutex
	cond   *sync.Cond
	conn   transport.Connection
	closed bool

	// redial, if non-nil, is invoked from a background goroutine after
	// Disconnect to obtain a fresh connection. The slot calls Set with the
	// returned connection so blocked Read/Write calls resume automatically.
	redial    func() (transport.Connection, error)
	redialing bool
}

var _ transport.Connection = (*ConnSlot)(nil)

// NewConnSlot wraps an initial connection. If redial is non-nil it is
// invoked from a background goroutine after a Disconnect or read/write
// error to obtain a fresh connection.
func NewConnSlot(conn transport.Connection, redial func() (transport.Connection, error)) *ConnSlot {
	s := &ConnSlot{conn: conn, redial: redial}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// IsAlive reports whether the slot has not been permanently closed.
func (s *ConnSlot) IsAlive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed
}

// Set installs a new connection, closing any previous one, and wakes
// goroutines blocked in Read or Write. If the slot is already closed, c is
// closed and the slot remains empty.
func (s *ConnSlot) Set(c transport.Connection) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		if c != nil {
			_ = c.Close()
		}
		return
	}
	prev := s.conn
	s.conn = c
	s.cond.Broadcast()
	s.mu.Unlock()
	if prev != nil {
		_ = prev.Close()
	}
}

// Disconnect closes the current connection but keeps the slot open.
// Subsequent Read and Write calls block until Set is called or Close is
// called. If a redialer was installed, a background goroutine is kicked off
// to obtain a fresh connection. Safe to call repeatedly and on closed slots.
func (s *ConnSlot) Disconnect() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	prev := s.conn
	s.conn = nil
	needReconnect := s.redial != nil && !s.redialing
	if needReconnect {
		s.redialing = true
	}
	s.mu.Unlock()
	if prev != nil {
		_ = prev.Close()
	}
	if needReconnect {
		go s.runRedial()
	}
}

// runRedial loops the redialer up to maxRedialAttempts times, installing
// the first successful connection via Set. On exhaustion the goroutine
// exits leaving the slot empty; producers stay back-pressured until Close.
func (s *ConnSlot) runRedial() {
	defer func() {
		s.mu.Lock()
		s.redialing = false
		s.mu.Unlock()
	}()

	for attempt := 1; attempt <= maxRedialAttempts; attempt++ {
		s.mu.Lock()
		if s.closed || s.conn != nil {
			s.mu.Unlock()
			return
		}
		redial := s.redial
		s.mu.Unlock()
		if redial == nil {
			return
		}

		c, err := redial()
		if err == nil {
			logrus.WithField("attempt", attempt).Info("ConnSlot: redial succeeded")
			s.Set(c)
			return
		}
		logrus.WithError(err).WithField("attempt", attempt).Debug("ConnSlot: redial failed")
		time.Sleep(redialInterval)
	}
	logrus.WithField("attempts", maxRedialAttempts).
		Warn("ConnSlot: redial attempts exhausted; slot left empty")
}

// Close permanently closes the slot. Any blocked Read or Write returns io.EOF.
// Calling Close on an already-closed slot is a no-op.
func (s *ConnSlot) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	prev := s.conn
	s.conn = nil
	s.cond.Broadcast()
	s.mu.Unlock()
	if prev != nil {
		_ = prev.Close()
	}
	return nil
}

// Read implements transport.Connection. Blocks until a connection is
// available or the slot is closed. On read error other than EOF, drops the
// current connection so the next Read waits for a replacement.
func (s *ConnSlot) Read(p []byte) (int, error) {
	c, err := s.acquire()
	if err != nil {
		return 0, err
	}
	n, rerr := c.Read(p)
	if rerr != nil && !errors.Is(rerr, io.EOF) {
		s.dropIfCurrent(c)
	}
	return n, rerr
}

// Write implements transport.Connection. Loops until all bytes are written
// or the slot is closed; on connection failure, drops the conn and parks
// in acquire for a replacement before retrying the remaining bytes. This
// is the back-pressure path: while disconnected, the loop parks and the
// caller's upstream pipe fills, eventually blocking the producing process.
func (s *ConnSlot) Write(p []byte) (int, error) {
	written := 0
	for written < len(p) {
		c, err := s.acquire()
		if err != nil {
			return written, err
		}
		n, werr := c.Write(p[written:])
		written += n
		if werr != nil {
			s.dropIfCurrent(c)
		}
	}
	return written, nil
}

// CloseRead delegates to the underlying connection if one is set.
func (s *ConnSlot) CloseRead() error {
	s.mu.Lock()
	c := s.conn
	s.mu.Unlock()
	if c == nil {
		return nil
	}
	return c.CloseRead()
}

// CloseWrite delegates to the underlying connection if one is set.
func (s *ConnSlot) CloseWrite() error {
	s.mu.Lock()
	c := s.conn
	s.mu.Unlock()
	if c == nil {
		return nil
	}
	return c.CloseWrite()
}

// File returns the current connection's file descriptor. Returns an error
// if the slot is disconnected or closed.
func (s *ConnSlot) File() (*os.File, error) {
	c, err := s.acquire()
	if err != nil {
		return nil, err
	}
	return c.File()
}

func (s *ConnSlot) acquire() (transport.Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.conn == nil && !s.closed {
		s.cond.Wait()
	}
	if s.closed {
		return nil, io.EOF
	}
	return s.conn, nil
}

// dropIfCurrent clears s.conn only when it still equals c, then closes c
// and starts a redial if needed. The conn-equality check avoids racing
// with a concurrent Set that may have already installed a fresh conn.
func (s *ConnSlot) dropIfCurrent(c transport.Connection) {
	s.mu.Lock()
	if s.closed || s.conn != c {
		s.mu.Unlock()
		return
	}
	s.conn = nil
	needReconnect := s.redial != nil && !s.redialing
	if needReconnect {
		s.redialing = true
	}
	s.mu.Unlock()
	_ = c.Close()
	if needReconnect {
		go s.runRedial()
	}
}
