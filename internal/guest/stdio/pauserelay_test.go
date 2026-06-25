//go:build linux
// +build linux

package stdio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/guest/transport"
)

// fakeConn adapts a net.Conn (one end of net.Pipe) to transport.Connection
// (io.ReadWriteCloser + CloseRead + CloseWrite + File). net.Pipe has no
// half-close, so CloseRead/CloseWrite close the whole connection, which is
// sufficient for the relay's pause/resume and clean-close paths. File is never
// exercised by the relay copiers, so it returns an error.
type fakeConn struct {
	net.Conn
}

func (f *fakeConn) CloseRead() error  { return f.Close() }
func (f *fakeConn) CloseWrite() error { return f.Close() }
func (f *fakeConn) File() (*os.File, error) {
	return nil, fmt.Errorf("fakeConn does not support File")
}

var _ transport.Connection = (*fakeConn)(nil)

// closeSignalConn is a transport.Connection whose Read blocks until unblock is
// closed (then returns EOF) and whose Close/CloseRead/CloseWrite are no-ops that
// introduce NO cross-goroutine synchronization. The no-op closes are the whole
// point: TestPipeRelayWaitRaceWithTeardown needs Wait's read of ConnectionSet.In
// and the relay manager goroutine's write of it (s.In = nil in Close) to race on
// the struct field itself. A fakeConn would route both Wait's CloseRead and the
// teardown's Close through net.Pipe's internal mutex, whose happens-before edge
// would mask that field race from -race; a real vsock conn's CloseRead/Close are
// independent syscalls and add no such edge, which is the production condition
// this double reproduces.
type closeSignalConn struct {
	unblock chan struct{}
}

func (c *closeSignalConn) Read([]byte) (int, error)    { <-c.unblock; return 0, io.EOF }
func (c *closeSignalConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *closeSignalConn) Close() error                { return nil }
func (c *closeSignalConn) CloseRead() error            { return nil }
func (c *closeSignalConn) CloseWrite() error           { return nil }
func (c *closeSignalConn) File() (*os.File, error) {
	return nil, fmt.Errorf("closeSignalConn does not support File")
}

var _ transport.Connection = (*closeSignalConn)(nil)

// TestPipeRelayPauseResume verifies that a PipeRelay pauses (rather than tears
// down) when the bridge drops mid-copy, re-dials the stdio connection, resumes
// copying onto the new connection, and only completes Wait once the process
// stdio is truly finished.
func TestPipeRelayPauseResume(t *testing.T) {
	SetBridgeDown(false)
	defer SetBridgeDown(false)

	// Each redial hands the test the new host end of a fresh net.Pipe so it can
	// read what the resumed relay writes.
	redialedHosts := make(chan net.Conn, 4)
	var redial func() (*ConnectionSet, error)
	redial = func() (*ConnectionSet, error) {
		host, guest := net.Pipe()
		redialedHosts <- host
		return &ConnectionSet{Out: &fakeConn{Conn: guest}, redial: redial}, nil
	}

	host1, guest1 := net.Pipe()
	out1 := &fakeConn{Conn: guest1}
	set := &ConnectionSet{Out: out1, redial: redial}

	pr, err := NewPipeRelay(nil)
	if err != nil {
		t.Fatalf("NewPipeRelay: %v", err)
	}
	pr.ReplaceConnectionSet(set)
	pr.CloseUnusedPipes()
	// Capture the stdout write pipe before Start so the test never races the
	// relay manager's closePipes on the pr.pipes fields.
	stdoutW := pr.pipes[3]
	pr.Start()

	// Pre-pause: write "hello" through the relay and read it on the host.
	if _, err := stdoutW.Write([]byte("hello")); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	_ = host1.SetReadDeadline(time.Now().Add(5 * time.Second))
	got := make([]byte, 5)
	if _, err := io.ReadFull(host1, got); err != nil {
		t.Fatalf("read hello: %v", err)
	}
	if !bytes.Equal(got, []byte("hello")) {
		t.Fatalf("pre-pause got %q, want hello", got)
	}

	// Simulate a bridge drop: mark the bridge down, close the host stdio conn,
	// then nudge the copier so its next write hits the closed conn and pauses.
	// The "x" is read from the pipe but cannot be written to the dead conn, so
	// the relay must hold it and replay it on the re-dialed conn (no drop).
	SetBridgeDown(true)
	_ = out1.Close()
	if _, err := stdoutW.Write([]byte("x")); err != nil {
		t.Fatalf("write pause trigger: %v", err)
	}

	// The relay re-dials; grab the new host end.
	var host2 net.Conn
	select {
	case host2 = <-redialedHosts:
	case <-time.After(5 * time.Second):
		t.Fatal("relay did not redial after pause")
	}

	// Post-resume: write "world"; assert the held "x" is replayed first and
	// then "world" arrives on the new host conn (the in-flight byte is not
	// dropped across the migration pause).
	if _, err := stdoutW.Write([]byte("world")); err != nil {
		t.Fatalf("write world: %v", err)
	}
	_ = host2.SetReadDeadline(time.Now().Add(5 * time.Second))
	got2 := make([]byte, len("xworld"))
	if _, err := io.ReadFull(host2, got2); err != nil {
		t.Fatalf("read xworld: %v", err)
	}
	if !bytes.Equal(got2, []byte("xworld")) {
		t.Fatalf("post-resume got %q, want xworld", got2)
	}

	// Process exit: close the stdout write pipe and the host conn so the relay
	// finishes and Wait returns.
	_ = stdoutW.Close()
	_ = host2.Close()
	SetBridgeDown(false)

	waitDone := make(chan struct{})
	go func() {
		pr.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after process exit")
	}
}

// errTestWriteFail is the failure a dead host connection's Write returns after a
// live-migration bridge drop. Both the BEFORE baseline (failWriter) and the
// AFTER proofs (recordConn) raise it so the two behaviors are compared against
// the same simulated fault.
var errTestWriteFail = errors.New("stdio test: simulated dead-conn write failure")

// failWriter is a minimal io.Writer whose Write always fails immediately without
// accepting any bytes. It is the destination in TestIoCopyDropsInFlightBytes:
// io.Copy reads the source into its internal buffer, this Write fails, and the
// buffered bytes are discarded. That discard is the pre-fix in-flight drop the
// new copyOut prevents.
type failWriter struct{}

func (failWriter) Write(p []byte) (int, error) { return 0, errTestWriteFail }

// countingReader is a source over a fixed payload that deliberately does NOT
// implement io.WriterTo, forcing io.Copy down its generic
// read-into-internal-buffer path (the path whose buffer is lost on a write
// error; the real relay source, an *os.File pipe end, has no faster path to a
// plain connection either). It records how many bytes io.Copy has read so the
// test can prove the payload was drained into, and then dropped by, io.Copy.
type countingReader struct {
	data []byte
	off  int
	read int
}

func (r *countingReader) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	r.read += n
	return n, nil
}

// recordConn is a transport.Connection test double for driving copyOut directly.
// When writeErr is non-nil every Write fails with it (modeling a dead host conn
// after a bridge drop); if partialN > 0 the Write first accepts (and records)
// that many bytes before failing, modeling a true partial write where the socket
// took a prefix before the bridge dropped. When zeroWrite is true every Write
// returns (0, nil) to exercise writeAll's short-write guard. Otherwise Write
// records the bytes so a test can prove the retained in-flight remainder is
// replayed in order. Read returns EOF so copyOut's clean-close path completes;
// CloseRead/CloseWrite/Close are inert and File is never used by the relay
// copiers.
type recordConn struct {
	mu        sync.Mutex
	written   []byte
	writeErr  error
	zeroWrite bool
	partialN  int
}

func (c *recordConn) Read(p []byte) (int, error) { return 0, io.EOF }

func (c *recordConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.zeroWrite {
		return 0, nil
	}
	if c.writeErr != nil {
		// A partial write accepts (and records) the first partialN bytes as
		// socket-level loss, then fails; partialN == 0 is the immediate-failure
		// conn that accepts nothing.
		if c.partialN > 0 {
			n := c.partialN
			if n > len(p) {
				n = len(p)
			}
			c.written = append(c.written, p[:n]...)
			return n, c.writeErr
		}
		return 0, c.writeErr
	}
	c.written = append(c.written, p...)
	return len(p), nil
}

func (c *recordConn) Close() error      { return nil }
func (c *recordConn) CloseRead() error  { return nil }
func (c *recordConn) CloseWrite() error { return nil }
func (c *recordConn) File() (*os.File, error) {
	return nil, fmt.Errorf("recordConn does not support File")
}

// recorded returns a copy of the bytes written to the connection so far.
func (c *recordConn) recorded() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := make([]byte, len(c.written))
	copy(b, c.written)
	return b
}

var _ transport.Connection = (*recordConn)(nil)

// testPattern returns n bytes of a deterministic, position-dependent pattern so
// any gap, duplication, or reordering of relayed bytes is detectable on
// comparison. 251 is prime, so the pattern does not align with 256 or the copy
// buffer sizes.
func testPattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

// firstDiff returns the index of the first byte at which a and b differ, or -1
// when they are equal. It pinpoints where a relayed stream lost contiguity for
// a compact failure message instead of dumping kilobytes of %q.
func firstDiff(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}

// window returns up to 16 bytes of p starting at i (clamped to p's bounds) for
// compact %q logging around the point two byte streams diverge.
func window(p []byte, i int) []byte {
	if i < 0 {
		i = 0
	}
	if i > len(p) {
		i = len(p)
	}
	end := i + 16
	if end > len(p) {
		end = len(p)
	}
	return p[i:end]
}

// TestIoCopyDropsInFlightBytes is the BEFORE baseline. In isolation it
// demonstrates the in-flight drop the old io.Copy-based relay suffered on a
// bridge drop: io.Copy reads the whole payload from the source into its internal
// buffer, the destination Write then fails, and those buffered bytes are gone.
// Zero bytes reach the destination yet the source is fully drained, so the bytes
// are unrecoverable. copyOut (proved by TestCopyOutRetainsInFlightBytes) instead
// holds and replays exactly these bytes.
func TestIoCopyDropsInFlightBytes(t *testing.T) {
	payload := testPattern(5000)
	src := &countingReader{data: payload}

	n, err := io.Copy(failWriter{}, src)

	if !errors.Is(err, errTestWriteFail) {
		t.Fatalf("io.Copy err = %v, want %v", err, errTestWriteFail)
	}
	if n != 0 {
		t.Fatalf("io.Copy transferred %d bytes to the failed destination, want 0", n)
	}
	// The bytes were read out of the source into io.Copy's internal buffer and
	// then lost when the Write failed: the source is fully drained but nothing
	// landed at the destination. This is the dropped in-flight data.
	if src.read != len(payload) {
		t.Fatalf("source read %d bytes, want %d (io.Copy did not drain the in-flight buffer it then dropped)", src.read, len(payload))
	}
	if src.off != len(payload) {
		t.Fatalf("source has %d bytes still unread, want 0 (expected fully drained)", len(payload)-src.off)
	}
}

// TestCopyOutRetainsInFlightBytes is the AFTER proof at the copyOut unit level.
// It shows copyOut holds the in-flight remainder that io.Copy would have dropped
// (see TestIoCopyDropsInFlightBytes) and replays it intact, in order, on the
// re-dialed connection.
func TestCopyOutRetainsInFlightBytes(t *testing.T) {
	SetBridgeDown(true)
	defer SetBridgeDown(false)

	// > one read but < relayBufferSize, so the payload is a single in-flight
	// chunk held whole on the failed write.
	payload := testPattern(5000)
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer pr.Close()
	if _, err := pw.Write(payload); err != nil {
		t.Fatalf("preload pipe: %v", err)
	}

	// First copyOut: the conn's Write fails while the bridge is down, so copyOut
	// must pause and hand back the full payload it read but could not deliver.
	deadConn := &recordConn{writeErr: errTestWriteFail}
	held, paused := copyOut(deadConn, pr, "stdout", nil, true)
	if !paused {
		t.Fatal("copyOut paused = false, want true on a bridge-down write failure")
	}
	if !bytes.Equal(held, payload) {
		t.Fatalf("copyOut held %d in-flight bytes, want %d retained intact: got %q want %q",
			len(held), len(payload), held, payload)
	}
	if got := deadConn.recorded(); len(got) != 0 {
		t.Fatalf("dead conn received %d bytes, want 0 (nothing should reach a failed write): %q", len(got), got)
	}

	// Second copyOut: replay the held remainder onto a fresh recording conn. Run
	// it in a goroutine because, after the replay, copyOut blocks reading the
	// still-open pipe; closing the write end lets it reach clean shutdown.
	rec := &recordConn{}
	done := make(chan struct{})
	var held2 []byte
	var paused2 bool
	go func() {
		held2, paused2 = copyOut(rec, pr, "stdout", held, true)
		close(done)
	}()
	_ = pw.Close() // unblock the post-replay read with EOF so copyOut returns
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("second copyOut did not return after pipe close")
	}

	if paused2 {
		t.Fatal("second copyOut paused = true, want false (write succeeds on the fresh conn)")
	}
	if held2 != nil {
		t.Fatalf("second copyOut held %q, want nil (nothing left to retain)", held2)
	}
	got := rec.recorded()
	if !bytes.HasPrefix(got, payload) {
		t.Fatalf("re-dialed conn did not receive the replayed remainder first: got %q want prefix %q", got, payload)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("re-dialed conn received %q, want exactly the replayed payload %q", got, payload)
	}
}

// TestCopyOutRetainsAcrossDoublePause proves the retained remainder survives a
// re-dial that ALSO fails. A held buffer that cannot be replayed on the first
// re-dialed conn (bridge still down) must be re-held, not dropped or truncated,
// and is replayed once a working conn appears.
func TestCopyOutRetainsAcrossDoublePause(t *testing.T) {
	SetBridgeDown(true)
	defer SetBridgeDown(false)

	payload := testPattern(5000)

	// First re-dial still bridge-down: the replay of the held remainder fails,
	// so copyOut must hand the same bytes back to be held again. The reader is
	// never consulted because the pending replay fails before copyOut's read
	// loop.
	deadConn1 := &recordConn{writeErr: errTestWriteFail}
	held1, paused1 := copyOut(deadConn1, bytes.NewReader(nil), "stdout", payload, true)
	if !paused1 {
		t.Fatal("first re-dial copyOut paused = false, want true (write still failing)")
	}
	if !bytes.Equal(held1, payload) {
		t.Fatalf("first re-dial dropped/truncated the held remainder: got %q want %q", held1, payload)
	}

	// Second re-dial also bridge-down: the re-held remainder must again be
	// returned whole.
	deadConn2 := &recordConn{writeErr: errTestWriteFail}
	held2, paused2 := copyOut(deadConn2, bytes.NewReader(nil), "stdout", held1, true)
	if !paused2 {
		t.Fatal("second re-dial copyOut paused = false, want true (write still failing)")
	}
	if !bytes.Equal(held2, payload) {
		t.Fatalf("second re-dial dropped/truncated the re-held remainder: got %q want %q", held2, payload)
	}

	// Finally a working conn: the twice-held remainder replays intact and in
	// order.
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer pr.Close()
	rec := &recordConn{}
	done := make(chan struct{})
	var paused3 bool
	go func() {
		_, paused3 = copyOut(rec, pr, "stdout", held2, true)
		close(done)
	}()
	_ = pw.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("final copyOut did not return after pipe close")
	}
	if paused3 {
		t.Fatal("final copyOut paused = true, want false (write succeeds)")
	}
	if got := rec.recorded(); !bytes.Equal(got, payload) {
		t.Fatalf("final replay delivered %q, want the full twice-held payload %q", got, payload)
	}
}

// TestWriteAllShortWriteGuard verifies the hardening that a misbehaving
// Connection returning (0, nil) makes writeAll fail with io.ErrShortWrite
// instead of spinning forever, matching io.Copy's contract. It runs writeAll in
// a goroutine so a regression that drops the guard fails as a timeout rather
// than hanging the package.
func TestWriteAllShortWriteGuard(t *testing.T) {
	conn := &recordConn{zeroWrite: true}
	type result struct {
		n   int
		err error
	}
	res := make(chan result, 1)
	go func() {
		n, err := writeAll(conn, testPattern(64))
		res <- result{n, err}
	}()
	select {
	case r := <-res:
		if !errors.Is(r.err, io.ErrShortWrite) {
			t.Fatalf("writeAll on a (0, nil) conn err = %v, want io.ErrShortWrite", r.err)
		}
		if r.n != 0 {
			t.Fatalf("writeAll on a (0, nil) conn wrote %d bytes, want 0", r.n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writeAll did not return on a (0, nil) conn: the short-write guard is missing (infinite loop)")
	}
}

// TestPipeRelayRetainsBurstAcrossPause drives a multi-KB burst through a full
// PipeRelay, drops the bridge after the host has read only the first half, and
// asserts the re-dialed host receives the exact remaining bytes so the two host
// reads concatenate to the original burst with no gap, duplication, or
// reordering. This is the end-to-end proof that the in-flight bytes io.Copy
// dropped are now retained and replayed.
func TestPipeRelayRetainsBurstAcrossPause(t *testing.T) {
	SetBridgeDown(false)
	defer SetBridgeDown(false)

	const (
		burstLen = 16 * 1024
		splitAt  = 8 * 1024
	)
	payload := testPattern(burstLen)

	redialedHosts := make(chan net.Conn, 4)
	var redial func() (*ConnectionSet, error)
	redial = func() (*ConnectionSet, error) {
		host, guest := net.Pipe()
		redialedHosts <- host
		return &ConnectionSet{Out: &fakeConn{Conn: guest}, redial: redial}, nil
	}

	host1, guest1 := net.Pipe()
	out1 := &fakeConn{Conn: guest1}
	set := &ConnectionSet{Out: out1, redial: redial}

	pr, err := NewPipeRelay(nil)
	if err != nil {
		t.Fatalf("NewPipeRelay: %v", err)
	}
	pr.ReplaceConnectionSet(set)
	pr.CloseUnusedPipes()
	// Capture the stdout write pipe before Start so the test never races the
	// relay manager's closePipes on the pr.pipes fields.
	stdoutW := pr.pipes[3]
	pr.Start()

	// The process emits the whole burst at once. The os.Pipe buffers it (the
	// burst is smaller than a pipe's capacity), so this Write does not block on
	// the relay; a goroutine guards against any partial-buffer edge anyway.
	writeErr := make(chan error, 1)
	go func() {
		_, werr := stdoutW.Write(payload)
		writeErr <- werr
	}()

	// Read only the first half on the original host. net.Pipe is synchronous, so
	// this returns once the relay's in-flight write has delivered exactly splitAt
	// bytes; the relay is then blocked mid-write holding the remainder.
	_ = host1.SetReadDeadline(time.Now().Add(5 * time.Second))
	first := make([]byte, splitAt)
	if _, err := io.ReadFull(host1, first); err != nil {
		t.Fatalf("read first half on original host: %v", err)
	}

	// Drop the bridge: mark it down, then close the host conn so the relay's
	// blocked write fails. copyOut must hold the undelivered remainder and replay
	// it on the re-dialed conn rather than drop it (the old io.Copy bug).
	SetBridgeDown(true)
	_ = out1.Close()

	var host2 net.Conn
	select {
	case host2 = <-redialedHosts:
	case <-time.After(5 * time.Second):
		t.Fatal("relay did not redial after pause")
	}

	// The relay replays the held remainder onto the new host. Read exactly the
	// remaining bytes; io.ReadFull collects them across however many writes the
	// relay needs.
	_ = host2.SetReadDeadline(time.Now().Add(5 * time.Second))
	second := make([]byte, burstLen-splitAt)
	if _, err := io.ReadFull(host2, second); err != nil {
		t.Fatalf("read held remainder on re-dialed host: %v", err)
	}
	if werr := <-writeErr; werr != nil {
		t.Fatalf("process burst write: %v", werr)
	}

	// The held remainder must be exactly the bytes that follow the split point,
	// and the two host reads must reconstruct the original burst contiguously.
	if !bytes.Equal(second, payload[splitAt:]) {
		d := firstDiff(second, payload[splitAt:])
		t.Fatalf("held remainder differs from the dropped bytes at offset %d: got %q want %q",
			d, window(second, d), window(payload[splitAt:], d))
	}
	got := append(append([]byte{}, first...), second...)
	if !bytes.Equal(got, payload) {
		d := firstDiff(got, payload)
		t.Fatalf("relayed burst not contiguous across pause: got %d bytes want %d, first diff at %d: got %q want %q",
			len(got), len(payload), d, window(got, d), window(payload, d))
	}

	// Process exit: close stdout and the host conn so the relay finishes.
	_ = stdoutW.Close()
	_ = host2.Close()
	SetBridgeDown(false)

	waitDone := make(chan struct{})
	go func() {
		pr.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after process exit")
	}
}

// TestCopyOutHoldsPartialWriteRemainder pins the partial-loss boundary. The conn
// accepts the first K bytes of an in-flight chunk and then fails while the bridge
// is down (a true partial write, the socket having taken a prefix before the
// drop). copyOut must hold exactly the tail the socket never accepted: the K
// accepted bytes are socket-level loss the relay cannot recover, and only
// payload[K:] is retained for replay on the re-dialed conn.
func TestCopyOutHoldsPartialWriteRemainder(t *testing.T) {
	SetBridgeDown(true)
	defer SetBridgeDown(false)

	// A single in-flight chunk (< relayBufferSize) so copyOut issues exactly one
	// Write; the conn takes k bytes then fails.
	const k = 1500
	payload := testPattern(5000)
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer pr.Close()
	if _, err := pw.Write(payload); err != nil {
		t.Fatalf("preload pipe: %v", err)
	}

	conn := &recordConn{writeErr: errTestWriteFail, partialN: k}
	held, paused := copyOut(conn, pr, "stdout", nil, true)
	if !paused {
		t.Fatal("copyOut paused = false, want true on a bridge-down partial write")
	}
	// held must be exactly the bytes after the accepted prefix.
	want := payload[k:]
	if !bytes.Equal(held, want) {
		d := firstDiff(held, want)
		t.Fatalf("copyOut held the wrong partial-write remainder: got %d bytes want %d, first diff at %d: got %q want %q",
			len(held), len(want), d, window(held, d), window(want, d))
	}
	// The conn recorded exactly the first K bytes (the socket-level loss).
	if got := conn.recorded(); !bytes.Equal(got, payload[:k]) {
		d := firstDiff(got, payload[:k])
		t.Fatalf("conn recorded the wrong accepted prefix: got %d bytes want %d, first diff at %d: got %q want %q",
			len(got), k, d, window(got, d), window(payload[:k], d))
	}
}

// errTestReadFail is the read-side failure a dropped bridge conn returns. It is
// the conn READ error (distinct from a pipe WRITE error) that must make copyIn
// pause while the bridge is down, proving copyIn decides by which side failed.
var errTestReadFail = errors.New("stdio test: simulated bridge-drop conn read failure")

// blockUntilReader yields data once and then blocks on block (closed by the
// test) on any further Read. In TestCopyInFinishesOnStdinCloseWhileBridgeDown the
// write fails on the first chunk so the second Read is never reached; the block
// is a guard that turns a regression (looping instead of returning) into a
// timeout rather than a wrong pass, and ensures the only way copyIn can return is
// via the write side.
type blockUntilReader struct {
	data  []byte
	sent  bool
	block chan struct{}
}

func (r *blockUntilReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, r.data), nil
	}
	<-r.block
	return 0, io.EOF
}

// errReader returns err on every Read with no bytes, modeling a host conn whose
// read fails (a bridge drop) rather than a normal EOF.
type errReader struct{ err error }

func (r errReader) Read(p []byte) (int, error) { return 0, r.err }

// TestCopyInFinishesOnStdinCloseWhileBridgeDown is the gap the side-aware copyIn
// closes. With the bridge down, a process that closes its stdin (a pipe WRITE
// failure) must finish (paused=false), not be mistaken for a live-migration
// pause; a real bridge-drop conn READ error must still pause (paused=true).
// Together they prove copyIn decides by which side failed, not by a single
// collapsed error as the old io.Copy did.
func TestCopyInFinishesOnStdinCloseWhileBridgeDown(t *testing.T) {
	SetBridgeDown(true)
	defer SetBridgeDown(false)

	// Process closed stdin: the reader yields bytes (so a write is attempted)
	// then blocks, and the sink's Write fails (EPIPE). copyIn must return via the
	// write side as false (done); the block guarantees no read error or EOF can
	// supply the return instead.
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	r := &blockUntilReader{data: []byte("stdin-bytes"), block: block}

	done := make(chan bool, 1)
	go func() {
		// failWriter models the process having closed its stdin: every Write
		// returns the EPIPE-like errTestWriteFail.
		done <- copyIn(failWriter{}, r, "stdin", true)
	}()
	select {
	case paused := <-done:
		if paused {
			t.Fatalf("copyIn paused = %v on a process stdin-close write failure while bridge down, want false: a pipe write error must finish, not pause", paused)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("copyIn did not return on a stdin-close write failure: it must decide by the write side, not loop while the bridge is down")
	}

	// Real bridge-drop conn read error while bridge down: copyIn must pause so
	// the manager re-dials and the next call resumes on the fresh conn.
	if paused := copyIn(io.Discard, errReader{err: errTestReadFail}, "stdin", true); !paused {
		t.Fatalf("copyIn paused = %v on a bridge-down conn read error, want true: a conn read failure during migration must pause", paused)
	}
}

// TestPipeRelaySurvivesMultipleMigrations drives a full PipeRelay through several
// back-and-forth live-migration pauses in a row: each cycle reads a chunk on the
// current host, drops the bridge and holds a single in-flight byte, re-dials a
// fresh net.Pipe host, and resumes. It asserts the concatenation of what every
// successive host conn received reconstructs everything written, contiguously and
// in order, proving each migration holds and replays correctly and that no
// held-buffer state leaks from one migration into the next. Synchronization is
// net.Pipe's blocking reads plus the redial handoff channel; there are no sleeps.
func TestPipeRelaySurvivesMultipleMigrations(t *testing.T) {
	SetBridgeDown(false)
	defer SetBridgeDown(false)

	const cycles = 4 // > 3 back-and-forth migrations

	// Carve every chunk from one position-dependent pattern so the concatenation
	// of all host receptions must reproduce it byte-for-byte; any gap,
	// duplication, or reorder across the repeated pause/replay cycles surfaces as
	// a firstDiff.
	base := testPattern(8 * 1024)
	off := 0
	take := func(n int) []byte {
		s := base[off : off+n]
		off += n
		return s
	}

	redialedHosts := make(chan net.Conn, cycles+2)
	var redial func() (*ConnectionSet, error)
	redial = func() (*ConnectionSet, error) {
		host, guest := net.Pipe()
		redialedHosts <- host
		return &ConnectionSet{Out: &fakeConn{Conn: guest}, redial: redial}, nil
	}

	host, guest := net.Pipe()
	set := &ConnectionSet{Out: &fakeConn{Conn: guest}, redial: redial}

	pr, err := NewPipeRelay(nil)
	if err != nil {
		t.Fatalf("NewPipeRelay: %v", err)
	}
	pr.ReplaceConnectionSet(set)
	pr.CloseUnusedPipes()
	// Capture the stdout write pipe before Start so the test never races the
	// relay manager's closePipes on the pr.pipes fields.
	stdoutW := pr.pipes[3]
	pr.Start()

	// written accumulates every byte handed to the process stdout pipe; read
	// accumulates every byte the successive host conns deliver. They must match.
	var written, read []byte

	for i := 0; i < cycles; i++ {
		// Pre-pause chunk for this connection (a distinct length per cycle). On
		// every cycle after the first the host also leads with the single held
		// byte replayed from the prior pause, so the expected read is that byte
		// plus this chunk.
		pre := take(96 + i)
		if _, err := stdoutW.Write(pre); err != nil {
			t.Fatalf("cycle %d: write pre-pause chunk: %v", i, err)
		}
		written = append(written, pre...)

		expect := len(pre)
		if i > 0 {
			expect++ // leading replayed held byte from the previous pause
		}
		buf := make([]byte, expect)
		_ = host.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, err := io.ReadFull(host, buf); err != nil {
			t.Fatalf("cycle %d: read %d bytes on host: %v", i, expect, err)
		}
		read = append(read, buf...)

		// Drop the bridge and close this host so the relay's next write fails and
		// pauses. The single nudge byte is read from the pipe but cannot be
		// delivered, so the relay must hold it and replay it on the next conn.
		SetBridgeDown(true)
		_ = host.Close()
		nudge := take(1)
		if _, err := stdoutW.Write(nudge); err != nil {
			t.Fatalf("cycle %d: write held nudge byte: %v", i, err)
		}
		written = append(written, nudge...)

		select {
		case host = <-redialedHosts:
		case <-time.After(5 * time.Second):
			t.Fatalf("cycle %d: relay did not redial after pause", i)
		}
		SetBridgeDown(false)
	}

	// Final connection: read the last held byte plus a final chunk, then drive a
	// clean process exit and assert Wait returns.
	final := take(64)
	if _, err := stdoutW.Write(final); err != nil {
		t.Fatalf("write final chunk: %v", err)
	}
	written = append(written, final...)
	tail := make([]byte, 1+len(final)) // leading replayed held byte + final
	_ = host.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(host, tail); err != nil {
		t.Fatalf("read final held byte + chunk: %v", err)
	}
	read = append(read, tail...)

	_ = stdoutW.Close()
	_ = host.Close()
	SetBridgeDown(false)

	waitDone := make(chan struct{})
	go func() {
		pr.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after process exit")
	}

	// The concatenation of everything the successive host conns received must
	// reconstruct everything written, contiguously: no relay-level gap,
	// duplication, or reorder across the repeated migrations, and no held-buffer
	// state leaking from one migration into the next.
	if !bytes.Equal(read, written) {
		d := firstDiff(read, written)
		t.Fatalf("relayed stream not contiguous across %d migrations: read %d bytes want %d, first diff at %d: got %q want %q",
			cycles, len(read), len(written), d, window(read, d), window(written, d))
	}
}

// TestPipeRelayWaitRaceWithTeardown drives Wait concurrently with the relay
// manager goroutine's teardown so Wait's CloseRead on the stdin conn overlaps
// run's Close of the same ConnectionSet. Its whole value is under -race -count:
// before the fix, Wait read ConnectionSet.In outside the relay mutex while the
// manager goroutine nil'd and closed it outside the mutex (a torn read /
// nil-deref that panics the GCS goroutine = UVM DoS); the fix moves both the
// CloseRead and the conn Close under the relay mutex. The test asserts only that
// nothing panics and that Wait returns.
func TestPipeRelayWaitRaceWithTeardown(t *testing.T) {
	SetBridgeDown(false)
	defer SetBridgeDown(false)

	// In is a closeSignalConn so Wait's read of ConnectionSet.In and the manager
	// goroutine's write of it race on the field with no conn-level happens-before
	// edge to mask it (see closeSignalConn). The stdin copier blocks on the conn's
	// Read until the test closes stdinUnblock, ending it independently of Wait so
	// the teardown that follows runs concurrently with Wait's CloseRead. Out is a
	// fakeConn over net.Pipe whose copier ends on the stdout pipe EOF; there is no
	// redial closure, so a copier ending means process exit (run tears down).
	stdinUnblock := make(chan struct{})
	outHost, outGuest := net.Pipe()
	set := &ConnectionSet{In: &closeSignalConn{unblock: stdinUnblock}, Out: &fakeConn{Conn: outGuest}}

	pr, err := NewPipeRelay(nil)
	if err != nil {
		t.Fatalf("NewPipeRelay: %v", err)
	}
	pr.ReplaceConnectionSet(set)
	pr.CloseUnusedPipes()
	// Capture the stdout write pipe before Start so closing it (to end the
	// stdout copier) never races the manager's closePipes on pr.pipes.
	stdoutW := pr.pipes[3]
	pr.Start()

	// Release Wait and the teardown trigger together. The trigger ends both
	// copiers WITHOUT Wait's help (stdinUnblock gives the stdin copier EOF; the
	// stdout pipe close gives the stdout copier EOF), so run's teardown Close of
	// the set runs genuinely concurrently with Wait's CloseRead / read of the same
	// ConnectionSet.In rather than after it. That overlap is what -race needs to
	// observe the unsynchronized field accesses the fix removes.
	start := make(chan struct{})
	waitDone := make(chan struct{})
	go func() {
		<-start
		pr.Wait()
		close(waitDone)
	}()

	close(start)
	close(stdinUnblock) // EOF to the stdin copier (independent of Wait)
	_ = stdoutW.Close() // EOF to the stdout copier

	select {
	case <-waitDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Wait did not return: relay teardown overlapping Wait deadlocked")
	}

	// The relay closed the guest side of Out during teardown; close the host side
	// so the test owns its cleanup (double close is harmless).
	_ = outHost.Close()
}
