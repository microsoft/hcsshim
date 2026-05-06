//go:build linux

package stdio

import (
	"bytes"
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/guest/transport"
)

// fakeConn is a controllable transport.Connection backed by an in-memory
// buffer pair, used to exercise ConnSlot without touching real sockets.
type fakeConn struct {
	mu          sync.Mutex
	rd          *bytes.Buffer
	wr          *bytes.Buffer
	closed      bool
	failNextRW  error
	closeReadCh chan struct{}
}

func newFakeConn() *fakeConn {
	return &fakeConn{
		rd:          new(bytes.Buffer),
		wr:          new(bytes.Buffer),
		closeReadCh: make(chan struct{}),
	}
}

func (c *fakeConn) feedRead(b []byte) {
	c.mu.Lock()
	c.rd.Write(b)
	c.mu.Unlock()
}

func (c *fakeConn) failNext(err error) {
	c.mu.Lock()
	c.failNextRW = err
	c.mu.Unlock()
}

func (c *fakeConn) writtenBytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.wr.Bytes()...)
}

func (c *fakeConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *fakeConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return 0, io.EOF
	}
	if c.failNextRW != nil {
		err := c.failNextRW
		c.failNextRW = nil
		c.mu.Unlock()
		return 0, err
	}
	if c.rd.Len() == 0 {
		c.mu.Unlock()
		// Block until close or another write feeds data; simulate a live socket.
		<-c.closeReadCh
		return 0, io.EOF
	}
	n, err := c.rd.Read(p)
	c.mu.Unlock()
	return n, err
}

func (c *fakeConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, io.ErrClosedPipe
	}
	if c.failNextRW != nil {
		err := c.failNextRW
		c.failNextRW = nil
		return 0, err
	}
	return c.wr.Write(p)
}

func (c *fakeConn) Close() error {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.closeReadCh)
	}
	c.mu.Unlock()
	return nil
}

func (c *fakeConn) CloseRead() error        { return c.Close() }
func (c *fakeConn) CloseWrite() error       { return nil }
func (c *fakeConn) File() (*os.File, error) { return nil, errors.New("no file") }

var _ transport.Connection = (*fakeConn)(nil)

// -----------------------------------------------------------------------------
// Basic happy-path
// -----------------------------------------------------------------------------

func TestConnSlot_Write_PassThroughWhenConnected(t *testing.T) {
	c := newFakeConn()
	s := NewConnSlot(c, nil)

	n, err := s.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("Write got n=%d err=%v, want n=5 err=nil", n, err)
	}
	if got := string(c.writtenBytes()); got != "hello" {
		t.Fatalf("underlying conn got %q, want %q", got, "hello")
	}
}

func TestConnSlot_Write_BlocksWhileDisconnected_ResumesAfterSet(t *testing.T) {
	c1 := newFakeConn()
	s := NewConnSlot(c1, nil)
	s.Disconnect()

	done := make(chan error, 1)
	go func() {
		_, err := s.Write([]byte("queued"))
		done <- err
	}()

	select {
	case <-done:
		t.Fatal("Write returned before Set was called")
	case <-time.After(50 * time.Millisecond):
	}

	c2 := newFakeConn()
	s.Set(c2)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Write after reconnect err=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Write did not complete after Set")
	}
	if got := string(c2.writtenBytes()); got != "queued" {
		t.Fatalf("c2 got %q, want %q", got, "queued")
	}
}

func TestConnSlot_Write_DropsConnOnError_RetriesRemainingOnNewConn(t *testing.T) {
	c1 := newFakeConn()
	c1.failNext(io.ErrShortWrite)
	s := NewConnSlot(c1, nil)

	done := make(chan error, 1)
	go func() {
		_, err := s.Write([]byte("payload"))
		done <- err
	}()

	select {
	case <-done:
		t.Fatal("Write returned before reconnect")
	case <-time.After(50 * time.Millisecond):
	}

	c2 := newFakeConn()
	s.Set(c2)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Write after recovery err=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Write did not complete after Set")
	}
	if got := string(c2.writtenBytes()); got != "payload" {
		t.Fatalf("c2 got %q, want full payload", got)
	}
}

func TestConnSlot_Read_BlocksWhileDisconnected_ResumesAfterSet(t *testing.T) {
	s := NewConnSlot(newFakeConn(), nil)
	s.Disconnect()

	type readResult struct {
		buf []byte
		err error
	}
	done := make(chan readResult, 1)
	go func() {
		buf := make([]byte, 16)
		n, err := s.Read(buf)
		done <- readResult{buf: buf[:n], err: err}
	}()

	select {
	case <-done:
		t.Fatal("Read returned before Set")
	case <-time.After(50 * time.Millisecond):
	}

	c2 := newFakeConn()
	c2.feedRead([]byte("greetings"))
	s.Set(c2)

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Read err=%v", r.err)
		}
		if string(r.buf) != "greetings" {
			t.Fatalf("Read got %q, want %q", string(r.buf), "greetings")
		}
	case <-time.After(time.Second):
		t.Fatal("Read did not return after Set")
	}
}

// -----------------------------------------------------------------------------
// Lifecycle / idempotency
// -----------------------------------------------------------------------------

func TestConnSlot_Close_UnblocksWriteWithEOF(t *testing.T) {
	s := NewConnSlot(newFakeConn(), nil)
	s.Disconnect()

	done := make(chan error, 1)
	go func() {
		_, err := s.Write([]byte("never sent"))
		done <- err
	}()

	time.Sleep(20 * time.Millisecond)
	_ = s.Close()

	select {
	case err := <-done:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("Write after Close got err=%v, want io.EOF", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Write did not return after Close")
	}
}

func TestConnSlot_Set_ClosesPreviousConnection(t *testing.T) {
	c1 := newFakeConn()
	c2 := newFakeConn()
	s := NewConnSlot(c1, nil)

	s.Set(c2)

	if !c1.isClosed() {
		t.Fatal("Set did not close previous connection")
	}
	if c2.isClosed() {
		t.Fatal("Set must not close the new connection")
	}
}

func TestConnSlot_SetAfterClose_ClosesNewConn(t *testing.T) {
	s := NewConnSlot(newFakeConn(), nil)
	_ = s.Close()

	c := newFakeConn()
	s.Set(c)

	if !c.isClosed() {
		t.Fatal("Set on closed slot must close the new connection (otherwise we leak it)")
	}
}

func TestConnSlot_Disconnect_Idempotent(t *testing.T) {
	c := newFakeConn()
	s := NewConnSlot(c, nil)

	s.Disconnect()
	s.Disconnect() // must not panic / double-close
	s.Disconnect()

	if !c.isClosed() {
		t.Fatal("Disconnect did not close underlying conn")
	}
}

func TestConnSlot_Close_Idempotent(t *testing.T) {
	c := newFakeConn()
	s := NewConnSlot(c, nil)

	if err := s.Close(); err != nil {
		t.Fatalf("Close 1 err=%v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close 2 err=%v", err)
	}
}

func TestConnSlot_Disconnect_AfterClose_NoOp(t *testing.T) {
	s := NewConnSlot(newFakeConn(), nil)
	_ = s.Close()
	s.Disconnect() // must not start a runRedial goroutine
}

func TestConnSlot_Disconnect_NoRedialer_NoGoroutine(t *testing.T) {
	c := newFakeConn()
	s := NewConnSlot(c, nil)
	// nil redialer.

	s.Disconnect()
	// Slot stays empty until explicit Set; nothing else should happen.
	// We assert by giving any rogue redial goroutine a chance to run and
	// confirming the conn stayed nil.
	time.Sleep(20 * time.Millisecond)

	if !s.IsAlive() {
		t.Fatal("Disconnect closed the slot")
	}
}

// -----------------------------------------------------------------------------
// Redial behavior
// -----------------------------------------------------------------------------

func TestConnSlot_Redial_SuccessOnFirstAttempt(t *testing.T) {
	c2 := newFakeConn()
	var calls atomic.Int32
	s := NewConnSlot(newFakeConn(), func() (transport.Connection, error) {
		calls.Add(1)
		return c2, nil
	})
	s.Disconnect()

	// Wait for runRedial to land c2 via Set.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		n, err := s.Write([]byte("x"))
		if err == nil && n == 1 {
			break
		}
	}
	if got := string(c2.writtenBytes()); got != "x" {
		t.Fatalf("c2 got %q, want %q", got, "x")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("redial calls=%d, want 1", got)
	}
}

func TestConnSlot_Redial_AlwaysFailing_BoundedAttempts_LeavesSlotEmpty(t *testing.T) {
	// Redialer always fails. runRedial must stop after maxRedialAttempts
	// attempts (no infinite goroutine), but must NOT close the slot — a
	// later Disconnect() from the bridge reconnect loop should be able to
	// kick off a fresh runRedial. This is the recovery path for a
	// destination listener that comes back online after the first run gave
	// up.
	if testing.Short() {
		t.Skip("slow: takes ~maxRedialAttempts * redialInterval to exhaust")
	}

	var calls atomic.Int32
	s := NewConnSlot(newFakeConn(), func() (transport.Connection, error) {
		calls.Add(1)
		return nil, errors.New("nope")
	})
	defer s.Close()
	s.Disconnect()

	// Wait for runRedial to exhaust its bounded attempts.
	deadline := time.Now().Add(time.Duration(maxRedialAttempts+5) * redialInterval)
	for time.Now().Before(deadline) && calls.Load() < int32(maxRedialAttempts) {
		time.Sleep(redialInterval)
	}
	if got := calls.Load(); got != int32(maxRedialAttempts) {
		t.Fatalf("redial calls=%d, want %d", got, maxRedialAttempts)
	}

	// Slot must remain open so the next bridge cycle can revive it.
	if !s.IsAlive() {
		t.Fatal("slot self-closed; want still-open so next Disconnect can re-trigger redial")
	}

	// Verify next Disconnect restarts redial. The previous goroutine's
	// final per-attempt sleep can run for redialInterval after we observed
	// the last increment, so wait two intervals to ensure the deferred
	// redialing=false has executed.
	time.Sleep(2 * redialInterval)
	prev := calls.Load()
	s.Disconnect()
	deadline = time.Now().Add(2 * redialInterval)
	for time.Now().Before(deadline) && calls.Load() == prev {
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() <= prev {
		t.Fatalf("Disconnect after exhaustion did not kick a fresh redial (calls stuck at %d)", prev)
	}
}

// -----------------------------------------------------------------------------
// Concurrency / race detector
// -----------------------------------------------------------------------------

func TestConnSlot_Concurrent_DisconnectWithWrites(t *testing.T) {
	// Run Disconnect and many writers concurrently to exercise the lock
	// discipline. With -race enabled this catches lock ordering bugs.
	var redialCalls atomic.Int32
	c := newFakeConn()
	s := NewConnSlot(c, func() (transport.Connection, error) {
		redialCalls.Add(1)
		return newFakeConn(), nil
	})
	defer s.Close()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 4 writers spinning small writes.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := []byte("x")
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, _ = s.Write(buf)
			}
		}()
	}

	// Disconnect 50 times with small pauses.
	for i := 0; i < 50; i++ {
		s.Disconnect()
		time.Sleep(time.Millisecond)
	}

	close(stop)
	wg.Wait()
}

// -----------------------------------------------------------------------------
// Pipe relay integration (matches real PipeRelay usage)
// -----------------------------------------------------------------------------

func TestConnSlot_PipeRelayIntegration(t *testing.T) {
	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer pipeR.Close()
	defer pipeW.Close()

	c1 := newFakeConn()
	s := NewConnSlot(c1, nil)

	relayDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(s, pipeR)
		relayDone <- err
	}()

	if _, err := pipeW.Write([]byte("first")); err != nil {
		t.Fatalf("write first: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if string(c1.writtenBytes()) == "first" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := string(c1.writtenBytes()); got != "first" {
		t.Fatalf("c1 got %q, want %q", got, "first")
	}

	s.Disconnect()

	if _, err := pipeW.Write([]byte("second")); err != nil {
		t.Fatalf("write second: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if got := string(c1.writtenBytes()); got != "first" {
		t.Fatalf("c1 received bytes during disconnect: %q", got)
	}

	c2 := newFakeConn()
	s.Set(c2)

	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if string(c2.writtenBytes()) == "second" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := string(c2.writtenBytes()); got != "second" {
		t.Fatalf("c2 got %q after reconnect, want %q", got, "second")
	}

	pipeW.Close()
	select {
	case <-relayDone:
	case <-time.After(time.Second):
		t.Fatal("relay did not exit after pipe close")
	}
	_ = s.Close()
}
