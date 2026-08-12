//go:build linux
// +build linux

package transport

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
)

// fakeConn is a scriptable Connection: writeFn drives per-call behavior, and
// anything written with the default behavior is captured for assertions.
type fakeConn struct {
	writeFn func(p []byte) (int, error)
	written bytes.Buffer
	closed  bool
}

func (c *fakeConn) Write(p []byte) (int, error) {
	if c.writeFn != nil {
		return c.writeFn(p)
	}
	return c.written.Write(p)
}
func (c *fakeConn) Read([]byte) (int, error) { return 0, nil }
func (c *fakeConn) Close() error             { c.closed = true; return nil }
func (c *fakeConn) CloseRead() error         { return nil }
func (c *fakeConn) CloseWrite() error        { return nil }
func (c *fakeConn) File() (*os.File, error)  { return nil, nil }

// fakeTransport hands back scripted connections/errors on each dial, in order.
type fakeTransport struct {
	mu    sync.Mutex
	conns []Connection
	errs  []error
	dials int
}

func (t *fakeTransport) Dial(uint32) (Connection, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	i := t.dials
	t.dials++
	var conn Connection
	var err error
	if i < len(t.conns) {
		conn = t.conns[i]
	}
	if i < len(t.errs) {
		err = t.errs[i]
	}
	return conn, err
}

// A healthy stream forwards straight through and never re-dials.
func TestReconnectWritePassthrough(t *testing.T) {
	base := &fakeConn{}
	tp := &fakeTransport{}
	var stopped atomic.Bool
	c := NewReconnectConnection(tp, 1, base, &stopped)

	n, err := c.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("write = (%d, %v), want (5, nil)", n, err)
	}
	if tp.dials != 0 {
		t.Fatalf("dials = %d, want 0", tp.dials)
	}
	if got := base.written.String(); got != "hello" {
		t.Fatalf("delivered = %q, want %q", got, "hello")
	}
}

// A mid-write drop reconnects and re-sends the retained tail (the accepted-but-
// maybe-lost bytes) followed by the remainder, so nothing is lost.
func TestReconnectResumesAfterDrop(t *testing.T) {
	dropped := &fakeConn{writeFn: func(p []byte) (int, error) {
		return 2, syscall.EPIPE // "accepted" "he", then the host went away
	}}
	healthy := &fakeConn{}
	tp := &fakeTransport{conns: []Connection{healthy}, errs: []error{nil}}
	var stopped atomic.Bool
	c := NewReconnectConnection(tp, 7, dropped, &stopped)

	n, err := c.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("write = (%d, %v), want (5, nil)", n, err)
	}
	if !dropped.closed {
		t.Fatalf("dropped connection was not closed before reconnect")
	}
	if got := healthy.written.String(); got != "hello" {
		t.Fatalf("delivered after reconnect = %q, want %q (replayed \"he\" + \"llo\")", got, "hello")
	}
	if tp.dials != 1 {
		t.Fatalf("dials = %d, want 1", tp.dials)
	}
}

// A failed dial is retried until one succeeds (exercises the retry loop once).
func TestReconnectRetriesUntilDial(t *testing.T) {
	dropped := &fakeConn{writeFn: func(p []byte) (int, error) {
		return 0, syscall.ECONNRESET
	}}
	healthy := &fakeConn{}
	tp := &fakeTransport{
		conns: []Connection{nil, healthy},
		errs:  []error{errors.New("connection refused"), nil},
	}
	var stopped atomic.Bool
	c := NewReconnectConnection(tp, 1, dropped, &stopped)

	n, err := c.Write([]byte("hi"))
	if err != nil || n != 2 {
		t.Fatalf("write = (%d, %v), want (2, nil)", n, err)
	}
	if got := healthy.written.String(); got != "hi" {
		t.Fatalf("delivered = %q, want %q", got, "hi")
	}
	if tp.dials != 2 {
		t.Fatalf("dials = %d, want 2", tp.dials)
	}
}

// A non-disconnect error surfaces to the caller and never triggers a re-dial.
func TestReconnectNonDisconnectErrorPassesThrough(t *testing.T) {
	boom := errors.New("boom")
	base := &fakeConn{writeFn: func(p []byte) (int, error) { return 0, boom }}
	tp := &fakeTransport{}
	var stopped atomic.Bool
	c := NewReconnectConnection(tp, 1, base, &stopped)

	if _, err := c.Write([]byte("x")); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if tp.dials != 0 {
		t.Fatalf("dials = %d, want 0", tp.dials)
	}
}

// Once teardown is signalled, a drop surfaces the error instead of looping.
func TestReconnectStopAbortsRedial(t *testing.T) {
	base := &fakeConn{writeFn: func(p []byte) (int, error) { return 0, syscall.EPIPE }}
	tp := &fakeTransport{}
	var stopped atomic.Bool
	stopped.Store(true)
	c := NewReconnectConnection(tp, 1, base, &stopped)

	n, err := c.Write([]byte("x"))
	if err == nil || n != 0 {
		t.Fatalf("write = (%d, %v), want (0, non-nil)", n, err)
	}
	if tp.dials != 0 {
		t.Fatalf("dials = %d, want 0 (must not dial once stopped)", tp.dials)
	}
}

// Half-close/close operations act on the underlying connection.
func TestReconnectCloseDelegates(t *testing.T) {
	base := &fakeConn{}
	tp := &fakeTransport{}
	var stopped atomic.Bool
	c := NewReconnectConnection(tp, 1, base, &stopped)

	if err := c.Close(); err != nil {
		t.Fatalf("close = %v, want nil", err)
	}
	if !base.closed {
		t.Fatalf("close was not delegated to the underlying connection")
	}
}

// Only genuine peer-gone errors are treated as reconnectable.
func TestIsDisconnectErr(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{syscall.EPIPE, true},
		{syscall.ECONNRESET, true},
		{syscall.ENOTCONN, true},
		{fmt.Errorf("wrapped: %w", syscall.EPIPE), true},
		{syscall.ECONNABORTED, false},
		{syscall.ESHUTDOWN, false},
		{errors.New("plain"), false},
		{nil, false},
	}
	for _, tc := range cases {
		if got := isDisconnectErr(tc.err); got != tc.want {
			t.Errorf("isDisconnectErr(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

// Bytes a connection accepted before it dropped are replayed on the new
// connection, so nothing is lost even though the guest advanced past them.
func TestReconnectReplaysLostBytes(t *testing.T) {
	var down atomic.Bool
	conn1 := &fakeConn{}
	conn1.writeFn = func(p []byte) (int, error) {
		if down.Load() {
			return 0, syscall.EPIPE
		}
		return conn1.written.Write(p) // accepts the bytes (host may never read them)
	}
	conn2 := &fakeConn{}
	tp := &fakeTransport{conns: []Connection{conn2}, errs: []error{nil}}
	var stopped atomic.Bool
	c := NewReconnectConnection(tp, 1, conn1, &stopped)

	if n, err := c.Write([]byte("AAA")); err != nil || n != 3 {
		t.Fatalf("first write = (%d, %v), want (3, nil)", n, err)
	}
	down.Store(true) // conn1's buffered "AAA" is lost and the next write drops
	if n, err := c.Write([]byte("BBB")); err != nil || n != 3 {
		t.Fatalf("second write = (%d, %v), want (3, nil)", n, err)
	}
	if got := conn2.written.String(); got != "AAABBB" {
		t.Fatalf("delivered = %q, want %q (lost \"AAA\" must be replayed)", got, "AAABBB")
	}
}

// replayBuffer keeps only the most recent size bytes, in write order.
func TestReplayBuffer(t *testing.T) {
	r := newReplayBuffer(4)
	if got := string(r.bytes()); got != "" {
		t.Fatalf("empty = %q, want \"\"", got)
	}
	r.append([]byte("ab"))
	if got := string(r.bytes()); got != "ab" {
		t.Fatalf("after ab = %q, want ab", got)
	}
	r.append([]byte("cd")) // exactly full
	if got := string(r.bytes()); got != "abcd" {
		t.Fatalf("after abcd = %q, want abcd", got)
	}
	r.append([]byte("ef")) // wraps, drops "ab"
	if got := string(r.bytes()); got != "cdef" {
		t.Fatalf("after wrap = %q, want cdef", got)
	}
	r.append([]byte("XYZ123")) // larger than size: keep last 4
	if got := string(r.bytes()); got != "Z123" {
		t.Fatalf("after oversized = %q, want Z123", got)
	}
}
