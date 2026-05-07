//go:build windows

package hcs

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/computecore"
	"github.com/Microsoft/hcsshim/internal/vmcompute"
)

// swapFunc replaces *target with fn for the duration of t, restoring the original on cleanup.
func swapFunc[T any](t *testing.T, target *T, fn T) {
	t.Helper()
	orig := *target
	*target = fn
	t.Cleanup(func() { *target = orig })
}

// setupCallback registers a fake callback on sys so the notification channels exist.
// Returns the callback number for firing test notifications.
func setupCallback(t *testing.T, sys *System) uintptr {
	t.Helper()
	swapFunc(t, &hcsRegisterComputeSystemCallback, func(_ context.Context, _ vmcompute.HcsSystem, _ uintptr, _ uintptr) (vmcompute.HcsCallback, error) {
		return vmcompute.HcsCallback(99), nil
	})
	if err := registerCallbackForTest(sys); err != nil {
		t.Fatalf("registerCallback: %v", err)
	}
	return sys.callbackNumber
}

// startMocks holds fake implementations of the four computecore calls used by Start.
// Tests build a startMocks, then call install(t) to swap the package-level vars
// for the duration of the test.
type startMocks struct {
	createOp    computecore.HcsOperation
	createErr   error
	createCalls int

	closeCalls int

	startErr   error
	startCalls int

	waitResult string
	waitErr    error
	waitCalls  int
}

func (m *startMocks) install(t *testing.T) {
	t.Helper()
	swapFunc(t, &hcsCreateOperation, func(_ context.Context, _ uintptr, _ uintptr) (computecore.HcsOperation, error) {
		m.createCalls++
		return m.createOp, m.createErr
	})
	swapFunc(t, &hcsCloseOperation, func(_ context.Context, _ computecore.HcsOperation) {
		m.closeCalls++
	})
	swapFunc(t, &hcsStartComputeSystem, func(_ context.Context, _ computecore.HcsSystem, _ computecore.HcsOperation, _ string) error {
		m.startCalls++
		return m.startErr
	})
	swapFunc(t, &hcsWaitForOperationResult, func(_ context.Context, _ computecore.HcsOperation, _ uint32) (string, error) {
		m.waitCalls++
		return m.waitResult, m.waitErr
	})
}

// TestStart_Success verifies the happy path: CreateOperation, StartComputeSystem,
// and WaitForOperationResult all succeed; CloseOperation runs via defer.
func TestStart_Success(t *testing.T) {
	sys := newTestSystemWithHandle("start-success", 42)
	m := &startMocks{createOp: 99}
	m.install(t)

	if err := sys.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if m.createCalls != 1 || m.startCalls != 1 || m.waitCalls != 1 || m.closeCalls != 1 {
		t.Errorf("call counts: create=%d start=%d wait=%d close=%d (want 1 each)",
			m.createCalls, m.startCalls, m.waitCalls, m.closeCalls)
	}
	if sys.startTime.IsZero() {
		t.Error("startTime should be set after successful Start")
	}
}

// TestStart_AlreadyClosed verifies that Start on a system whose handle has been
// cleared returns ErrAlreadyClosed without invoking any HCS API.
func TestStart_AlreadyClosed(t *testing.T) {
	sys := newTestSystemWithHandle("start-closed", 0)
	m := &startMocks{}
	m.install(t)

	err := sys.Start(context.Background())
	if !errors.Is(err, ErrAlreadyClosed) {
		t.Fatalf("expected ErrAlreadyClosed, got: %v", err)
	}
	if m.createCalls != 0 || m.startCalls != 0 || m.waitCalls != 0 {
		t.Errorf("no HCS calls expected for closed handle: create=%d start=%d wait=%d",
			m.createCalls, m.startCalls, m.waitCalls)
	}
}

// TestStart_CreateOperationFails verifies the failure surfaces and Close is not
// called (no operation handle was acquired).
func TestStart_CreateOperationFails(t *testing.T) {
	sys := newTestSystemWithHandle("start-create-fail", 42)
	m := &startMocks{createErr: errors.New("create-op-failed")}
	m.install(t)

	err := sys.Start(context.Background())
	if err == nil || !errors.Is(err, m.createErr) {
		t.Fatalf("expected create error to surface, got: %v", err)
	}
	if m.startCalls != 0 || m.waitCalls != 0 || m.closeCalls != 0 {
		t.Errorf("no further calls expected after create failure: start=%d wait=%d close=%d",
			m.startCalls, m.waitCalls, m.closeCalls)
	}
}

// TestStart_StartCallFails verifies that a failure from HcsStartComputeSystem
// surfaces, Wait is skipped, but Close is still invoked via defer.
func TestStart_StartCallFails(t *testing.T) {
	sys := newTestSystemWithHandle("start-call-fail", 42)
	m := &startMocks{createOp: 7, startErr: errors.New("start-call-failed")}
	m.install(t)

	err := sys.Start(context.Background())
	if err == nil || !errors.Is(err, m.startErr) {
		t.Fatalf("expected start-call error, got: %v", err)
	}
	if m.waitCalls != 0 {
		t.Errorf("wait must not be called when start fails (got %d)", m.waitCalls)
	}
	if m.closeCalls != 1 {
		t.Errorf("close must run via defer even on start failure (got %d)", m.closeCalls)
	}
	if !sys.startTime.IsZero() {
		t.Error("startTime must remain zero after failed Start")
	}
}

// TestStart_WaitReturnsContainerExit simulates the VM exiting during boot:
// HcsWaitForOperationResult returns ErrUnexpectedContainerExit, which Start
// must surface unchanged.
func TestStart_WaitReturnsContainerExit(t *testing.T) {
	sys := newTestSystemWithHandle("start-vm-exit", 42)
	m := &startMocks{createOp: 1, waitErr: ErrUnexpectedContainerExit}
	m.install(t)

	err := sys.Start(context.Background())
	if !errors.Is(err, ErrUnexpectedContainerExit) {
		t.Fatalf("expected ErrUnexpectedContainerExit, got: %v", err)
	}
	if m.closeCalls != 1 {
		t.Errorf("close must run via defer (got %d)", m.closeCalls)
	}
}

// TestStart_WaitReturnsServiceAbort simulates the HCS service disconnecting
// during boot: HcsWaitForOperationResult returns ErrUnexpectedProcessAbort.
func TestStart_WaitReturnsServiceAbort(t *testing.T) {
	sys := newTestSystemWithHandle("start-svc-abort", 42)
	m := &startMocks{createOp: 1, waitErr: ErrUnexpectedProcessAbort}
	m.install(t)

	err := sys.Start(context.Background())
	if !errors.Is(err, ErrUnexpectedProcessAbort) {
		t.Fatalf("expected ErrUnexpectedProcessAbort, got: %v", err)
	}
	if m.closeCalls != 1 {
		t.Errorf("close must run via defer (got %d)", m.closeCalls)
	}
}

// TestStart_WaitReturnsTimeout simulates HCS reporting a timeout from the
// operation wait. Start must surface the syscall.Errno from HCS.
func TestStart_WaitReturnsTimeout(t *testing.T) {
	sys := newTestSystemWithHandle("start-timeout", 42)
	waitErr := syscall.Errno(0x000005B4) // ERROR_TIMEOUT
	m := &startMocks{createOp: 1, waitErr: waitErr}
	m.install(t)

	err := sys.Start(context.Background())
	if !errors.Is(err, waitErr) {
		t.Fatalf("expected wait timeout to surface, got: %v", err)
	}
	if m.closeCalls != 1 {
		t.Errorf("close must run via defer (got %d)", m.closeCalls)
	}
}

// TestPause_SystemExitedDuringPending verifies that if the VM exits while Pause
// is waiting for PauseCompleted, the caller gets ErrUnexpectedContainerExit.
func TestPause_SystemExitedDuringPending(t *testing.T) {
	sys := newTestSystemWithHandle("pause-exit", 42)
	cbNum := setupCallback(t, sys)

	swapFunc(t, &hcsPauseComputeSystem, func(_ context.Context, _ vmcompute.HcsSystem, _ string) (string, error) {
		return "", syscall.Errno(0xC0370103)
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		fireNotificationForTest(cbNum, hcsNotificationSystemExited, nil)
	}()

	err := sys.Pause(context.Background())
	if !errors.Is(err, ErrUnexpectedContainerExit) {
		t.Fatalf("expected ErrUnexpectedContainerExit, got: %v", err)
	}
}

// TestWaitBackground_NormalExit verifies that when SystemExited fires with nil
// error, Wait returns nil and exitError stays nil. This is the clean shutdown path.
func TestWaitBackground_NormalExit(t *testing.T) {
	sys := newTestSystemWithHandle("wait-normal", 42)
	cbNum := setupCallback(t, sys)
	startWaitBackgroundForTest(sys)

	time.Sleep(20 * time.Millisecond)
	fireNotificationForTest(cbNum, hcsNotificationSystemExited, nil)

	if err := sys.Wait(); err != nil {
		t.Fatalf("expected nil from Wait, got: %v", err)
	}
	if sys.exitError != nil {
		t.Fatalf("expected nil exitError, got: %v", sys.exitError)
	}
}

// TestWaitBackground_UnexpectedExit verifies that when SystemExited fires with
// ErrVmcomputeUnexpectedExit, waitError is nil but exitError captures the crash.
// This distinction matters: Wait() callers get nil (the system did stop), but
// ExitError() reveals it was abnormal.
func TestWaitBackground_UnexpectedExit(t *testing.T) {
	sys := newTestSystemWithHandle("wait-unexpected", 42)
	cbNum := setupCallback(t, sys)
	startWaitBackgroundForTest(sys)

	time.Sleep(20 * time.Millisecond)
	fireNotificationForTest(cbNum, hcsNotificationSystemExited, syscall.Errno(0xC0370106))

	if err := sys.Wait(); err != nil {
		t.Fatalf("expected nil from Wait, got: %v", err)
	}
	if sys.exitError == nil {
		t.Fatal("expected non-nil exitError after unexpected exit")
	}
	if !errors.Is(sys.exitError, syscall.Errno(0xC0370106)) {
		t.Fatalf("exitError should wrap ErrVmcomputeUnexpectedExit, got: %v", sys.exitError)
	}
}

// TestWait_MultipleGoroutines verifies that multiple goroutines blocked on Wait
// all unblock when the system exits. This tests the channel fan-out via waitBlock.
func TestWait_MultipleGoroutines(t *testing.T) {
	sys := newTestSystemWithHandle("wait-fanout", 42)
	cbNum := setupCallback(t, sys)
	startWaitBackgroundForTest(sys)

	const numWaiters = 5
	results := make(chan error, numWaiters)
	for i := 0; i < numWaiters; i++ {
		go func() { results <- sys.Wait() }()
	}

	time.Sleep(50 * time.Millisecond)
	fireNotificationForTest(cbNum, hcsNotificationSystemExited, nil)

	for i := 0; i < numWaiters; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Errorf("waiter %d: expected nil, got: %v", i, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("waiter %d timed out", i)
		}
	}
}

// TestCallback_LateNotificationAfterUnregister verifies that firing a
// notification after unregisterCallback has cleaned up does not panic.
// The callbackMap entry is deleted and channels are closed; a late fire
// should be a no-op.
func TestCallback_LateNotificationAfterUnregister(t *testing.T) {
	sys := newTestSystemWithHandle("late-callback", 42)
	cbNum := setupCallback(t, sys)

	if _, ok := callbackMap[cbNum]; !ok {
		t.Fatal("callback should exist after registration")
	}

	swapFunc(t, &hcsUnregisterComputeSystemCallback, func(_ context.Context, _ vmcompute.HcsCallback) error {
		return nil
	})
	if err := sys.unregisterCallback(context.Background()); err != nil {
		t.Fatalf("unregisterCallback: %v", err)
	}

	callbackMapLock.RLock()
	_, exists := callbackMap[cbNum]
	callbackMapLock.RUnlock()
	if exists {
		t.Fatal("callback should not exist after unregistration")
	}

	// Late fires — must not panic.
	fireNotificationForTest(cbNum, hcsNotificationSystemExited, nil)
	fireNotificationForTest(cbNum, hcsNotificationSystemStartCompleted, nil)
}
