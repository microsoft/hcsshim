//go:build linux
// +build linux

package stdio

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

// newTestConsole allocates a pty master through the same ioctls NewConsole uses.
//
// NewConsole itself is not called here because it chowns the slave to root,
// which needs privileges the test runner does not have. The chown is incidental;
// the ioctls are what decide whether the master stays pollable.
func newTestConsole(t *testing.T) *os.File {
	t.Helper()

	master, err := os.OpenFile("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		// A sandbox without a devpts mount cannot host these tests at all. Skip
		// rather than fail: this says nothing about the code under test.
		t.Skipf("cannot allocate a pty (%v); skipping", err)
	}
	t.Cleanup(func() { _ = master.Close() })

	if _, err := ptsname(master); err != nil {
		t.Fatalf("ptsname: %v", err)
	}
	if err := unlockpt(master); err != nil {
		t.Fatalf("unlockpt: %v", err)
	}
	return master
}

// readDeadlineWorks reports whether an elapsed read deadline actually interrupts
// a blocking read on f.
//
// It cannot just check SetReadDeadline's error: a file that is not registered
// with the runtime poller accepts the deadline, returns nil, and ignores it. The
// only reliable check is to park in a read and see whether it comes back.
func readDeadlineWorks(t *testing.T, f *os.File) bool {
	t.Helper()

	if err := f.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 8)
		_, err := f.Read(buf)
		done <- err
	}()

	select {
	case err := <-done:
		return errors.Is(err, os.ErrDeadlineExceeded)
	case <-time.After(5 * time.Second):
		return false
	}
}

// TestConsoleMasterStaysPollable verifies the pty master can still be
// interrupted by a read deadline after the console setup and resize ioctls.
//
// This is what makes TtyRelay recoverable: an idle terminal parks its stdout
// copier in a read that yields neither data nor EOF while the container runs,
// and runCopiers waits for every copier, so without an interruptible read the
// relay can never reach its redial after a live migration.
//
// Any use of os.File.Fd on the master breaks this. Fd reverts the descriptor to
// blocking mode and drops it from the runtime poller permanently, after which
// SetReadDeadline silently does nothing, which is why the ioctls go through
// SyscallConn.
func TestConsoleMasterStaysPollable(t *testing.T) {
	master := newTestConsole(t)

	if !readDeadlineWorks(t, master) {
		t.Fatal("read deadline did not interrupt a read on a fresh pty master; TtyRelay cannot be woken after a migration")
	}

	// ResizeConsole runs at any point in a container's life, so it must not cost
	// the master its pollability either.
	if err := ResizeConsole(master, 24, 80); err != nil {
		t.Fatalf("ResizeConsole: %v", err)
	}
	if !readDeadlineWorks(t, master) {
		t.Fatal("read deadline stopped working after ResizeConsole; the resize ioctl unregistered the master from the poller")
	}
}

// TestTtyRelayRedialsOnBridgeReconnectWhileIdle verifies a TTY relay re-dials on
// the bridge event alone, with a real pty and no traffic on any stream.
//
// This is the live-migration case for an interactive container: on the
// destination every stdio connection is dead, but a silent terminal produces
// nothing, so no copier ever fails and the write-failure path cannot fire.
func TestTtyRelayRedialsOnBridgeReconnectWhileIdle(t *testing.T) {
	master := newTestConsole(t)

	redialed := make(chan struct{}, 4)
	var redial func() (*ConnectionSet, error)
	redial = func() (*ConnectionSet, error) {
		redialed <- struct{}{}
		return &ConnectionSet{Out: &recordConn{}, redial: redial}, nil
	}

	// No write ever fails and nothing is written to the pty: the bridge
	// notification is the only thing that can start a redial.
	r := NewTtyRelay(&ConnectionSet{Out: &recordConn{}, redial: redial}, master)
	r.Start()

	NotifyBridgeReconnected()

	select {
	case <-redialed:
	case <-time.After(5 * time.Second):
		t.Fatal("idle tty relay did not redial after the bridge reconnected")
	}

	// Stand in for process exit: closing the master ends the stdout copier with
	// a non-redialable error, so run finishes. teardown closing it a second time
	// is harmless.
	_ = master.Close()
	waitDone := make(chan struct{})
	go func() {
		r.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after the pty closed")
	}
}
