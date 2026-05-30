//go:build windows && (lcow || wcow)

package vm

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/go-winio/pkg/process"
	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/controller/vm/mocks"
	"github.com/Microsoft/hcsshim/internal/gcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	iwin "github.com/Microsoft/hcsshim/internal/windows"

	"go.uber.org/mock/gomock"
	"golang.org/x/sys/windows"
)

// ─── helpers ───────────────────────────────────────────────────────────────────

// newControllerWithState returns a Controller wired to gomock uvm/guest mocks
// in the given state. The caller can set expectations on the returned mocks.
func newControllerWithState(t *testing.T, state State) (*Controller, *mocks.MockutilityVM, *mocks.MockguestManager) {
	t.Helper()
	ctrl := gomock.NewController(t)
	uvm := mocks.NewMockutilityVM(ctrl)
	guest := mocks.NewMockguestManager(ctrl)
	c := &Controller{
		uvm:           uvm,
		guest:         guest,
		vmState:       state,
		logOutputDone: make(chan struct{}),
	}
	return c, uvm, guest
}

// stubCaps is a test-only implementation of [gcs.GuestDefinedCapabilities].
type stubCaps struct{ dumpStacksSupported bool }

func (s stubCaps) IsSignalProcessSupported() bool        { return false }
func (s stubCaps) IsDeleteContainerStateSupported() bool { return false }
func (s stubCaps) IsDumpStacksSupported() bool           { return s.dumpStacksSupported }
func (s stubCaps) IsNamespaceAddRequestSupported() bool  { return false }

var _ gcs.GuestDefinedCapabilities = stubCaps{}

// fakeListener satisfies net.Listener for testing hvsock setup.
type fakeListener struct{ closed atomic.Bool }

func (f *fakeListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (f *fakeListener) Close() error              { f.closed.Store(true); return nil }
func (f *fakeListener) Addr() net.Addr            { return nil }

// fakeConn satisfies net.Conn for testing hvsock connections.
type fakeConn struct{}

func (f *fakeConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (f *fakeConn) Write(b []byte) (int, error)      { return len(b), nil }
func (f *fakeConn) Close() error                     { return nil }
func (f *fakeConn) LocalAddr() net.Addr              { return nil }
func (f *fakeConn) RemoteAddr() net.Addr             { return nil }
func (f *fakeConn) SetDeadline(time.Time) error      { return nil }
func (f *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakeConn) SetWriteDeadline(time.Time) error { return nil }

// swapListenHVSock replaces the package-level listenHVSock for the duration of t.
func swapListenHVSock(t *testing.T, fn func(*winio.HvsockAddr) (net.Listener, error)) {
	t.Helper()
	orig := listenHVSock
	t.Cleanup(func() { listenHVSock = orig })
	listenHVSock = fn
}

// swapLookupVMMEM replaces the package-level lookupVMMEM for the duration of t.
func swapLookupVMMEM(t *testing.T, fn func(context.Context, guid.GUID, iwin.API) (windows.Handle, error)) {
	t.Helper()
	orig := lookupVMMEM
	t.Cleanup(func() { lookupVMMEM = orig })
	lookupVMMEM = fn
}

// swapGetProcessMemoryInfo replaces the package-level getProcessMemoryInfo for the duration of t.
func swapGetProcessMemoryInfo(t *testing.T, fn func(windows.Handle) (*process.ProcessMemoryCountersEx, error)) {
	t.Helper()
	orig := getProcessMemoryInfo
	t.Cleanup(func() { getProcessMemoryInfo = orig })
	getProcessMemoryInfo = fn
}

// testGUID is a fixed GUID for test assertions.
var testGUID = guid.GUID{Data1: 0xDEADBEEF}

// ─── 1. State Machine Guards ───────────────────────────────────────────────────

func TestStateGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateVM/already_created", func(t *testing.T) {
		c, _, _ := newControllerWithState(t, StateCreated)
		err := c.CreateVM(ctx, &CreateOptions{ID: "test-vm"})
		if err == nil {
			t.Error("expected error for CreateVM on already-Created controller")
		}
	})

	t.Run("StartVM/not_created", func(t *testing.T) {
		c, _, _ := newControllerWithState(t, StateNotCreated)
		err := c.StartVM(ctx, &StartOptions{})
		if err == nil {
			t.Error("expected error for StartVM on NotCreated controller")
		}
	})

	t.Run("StartVM/already_running_idempotent", func(t *testing.T) {
		c, _, _ := newControllerWithState(t, StateRunning)
		err := c.StartVM(ctx, &StartOptions{})
		if err != nil {
			t.Errorf("expected nil for StartVM on already-Running controller, got: %v", err)
		}
	})

	t.Run("TerminateVM/not_created_idempotent", func(t *testing.T) {
		c := New()
		err := c.TerminateVM(ctx)
		if err != nil {
			t.Errorf("expected nil for TerminateVM on NotCreated controller, got: %v", err)
		}
	})

	t.Run("TerminateVM/already_terminated_idempotent", func(t *testing.T) {
		c, _, _ := newControllerWithState(t, StateTerminated)
		err := c.TerminateVM(ctx)
		if err != nil {
			t.Errorf("expected nil for TerminateVM on Terminated controller, got: %v", err)
		}
	})

	// Table-driven: methods that require StateRunning.
	runningOnlyTests := []struct {
		name string
		call func(*Controller) error
	}{
		{
			name: "UpdateCPU",
			call: func(c *Controller) error {
				return c.UpdateCPU(ctx, &hcsschema.ProcessorLimits{})
			},
		},
		{
			name: "UpdateMemory",
			call: func(c *Controller) error {
				return c.UpdateMemory(ctx, 1024)
			},
		},
		{
			name: "UpdateCPUGroup",
			call: func(c *Controller) error {
				return c.UpdateCPUGroup(ctx, "some-group-id")
			},
		},
		{
			name: "UpdatePolicyFragment",
			call: func(c *Controller) error {
				return c.UpdatePolicyFragment(ctx, guestresource.SecurityPolicyFragment{})
			},
		},
		{
			name: "DumpStacks",
			call: func(c *Controller) error {
				_, err := c.DumpStacks(ctx)
				return err
			},
		},
		{
			name: "Stats",
			call: func(c *Controller) error {
				_, err := c.Stats(ctx)
				return err
			},
		},
		{
			name: "ExecIntoHost",
			call: func(c *Controller) error {
				_, err := c.ExecIntoHost(ctx, &shimdiag.ExecProcessRequest{Args: []string{"echo"}})
				return err
			},
		},
	}
	for _, tc := range runningOnlyTests {
		t.Run(tc.name+"/not_running", func(t *testing.T) {
			c, _, _ := newControllerWithState(t, StateCreated)
			if err := tc.call(c); err == nil {
				t.Errorf("expected error for %s on non-Running controller", tc.name)
			}
		})
	}

	t.Run("Wait/not_created", func(t *testing.T) {
		c := New()
		err := c.Wait(ctx)
		if err == nil {
			t.Error("expected error for Wait on NotCreated controller")
		}
	})

	t.Run("ExitStatus/not_terminated", func(t *testing.T) {
		c, _, _ := newControllerWithState(t, StateRunning)
		_, err := c.ExitStatus()
		if err == nil {
			t.Error("expected error for ExitStatus on non-Terminated controller")
		}
	})
}

// ─── 2. TerminateVM Cleanup Chain ──────────────────────────────────────────────

func TestTerminateVM(t *testing.T) {
	ctx := context.Background()

	t.Run("success/running_to_terminated", func(t *testing.T) {
		c, uvm, guest := newControllerWithState(t, StateRunning)

		uvm.EXPECT().Terminate(gomock.Any()).Return(nil)
		guest.EXPECT().CloseConnection().Return(nil)
		uvm.EXPECT().Close(gomock.Any()).Return(nil)

		if err := c.TerminateVM(ctx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.State() != StateTerminated {
			t.Errorf("expected StateTerminated, got %s", c.State())
		}
	})

	t.Run("close_connection_error_logged_not_returned", func(t *testing.T) {
		c, uvm, guest := newControllerWithState(t, StateRunning)

		uvm.EXPECT().Terminate(gomock.Any()).Return(nil)
		guest.EXPECT().CloseConnection().Return(errors.New("guest connection error"))
		uvm.EXPECT().Close(gomock.Any()).Return(nil)

		if err := c.TerminateVM(ctx); err != nil {
			t.Errorf("expected nil (CloseConnection error should be logged, not returned), got: %v", err)
		}
		if c.State() != StateTerminated {
			t.Errorf("expected StateTerminated, got %s", c.State())
		}
	})

	t.Run("close_fails/state_invalid", func(t *testing.T) {
		c, uvm, guest := newControllerWithState(t, StateRunning)

		uvm.EXPECT().Terminate(gomock.Any()).Return(nil)
		guest.EXPECT().CloseConnection().Return(nil)
		uvm.EXPECT().Close(gomock.Any()).Return(errors.New("close failed"))

		err := c.TerminateVM(ctx)
		if err == nil {
			t.Fatal("expected error when uvm.Close fails")
		}
		if c.State() != StateInvalid {
			t.Errorf("expected StateInvalid after Close failure, got %s", c.State())
		}
	})

	t.Run("double_terminate/second_is_noop", func(t *testing.T) {
		c, uvm, guest := newControllerWithState(t, StateRunning)

		// First terminate succeeds.
		uvm.EXPECT().Terminate(gomock.Any()).Return(nil)
		guest.EXPECT().CloseConnection().Return(nil)
		uvm.EXPECT().Close(gomock.Any()).Return(nil)

		if err := c.TerminateVM(ctx); err != nil {
			t.Fatalf("first TerminateVM: %v", err)
		}

		// Second call — no mock expectations needed; should return nil immediately.
		if err := c.TerminateVM(ctx); err != nil {
			t.Errorf("second TerminateVM should be nil, got: %v", err)
		}
	})

	t.Run("terminate_from_invalid/recovers", func(t *testing.T) {
		// Simulate: Close failed → StateInvalid, then TerminateVM again.
		c, uvm, guest := newControllerWithState(t, StateRunning)

		// First call: Close fails → Invalid.
		uvm.EXPECT().Terminate(gomock.Any()).Return(nil)
		guest.EXPECT().CloseConnection().Return(nil)
		uvm.EXPECT().Close(gomock.Any()).Return(errors.New("close failed"))

		_ = c.TerminateVM(ctx)
		if c.State() != StateInvalid {
			t.Fatalf("precondition: expected StateInvalid, got %s", c.State())
		}

		// Second call from Invalid: should attempt cleanup again.
		uvm.EXPECT().Terminate(gomock.Any()).Return(nil)
		guest.EXPECT().CloseConnection().Return(nil)
		uvm.EXPECT().Close(gomock.Any()).Return(nil)

		if err := c.TerminateVM(ctx); err != nil {
			t.Errorf("TerminateVM from Invalid should succeed, got: %v", err)
		}
		if c.State() != StateTerminated {
			t.Errorf("expected StateTerminated, got %s", c.State())
		}
	})
}

// ─── 3. StartVM Error Cascade ──────────────────────────────────────────────────

// setupStartVMEnv injects fakes for listenHVSock and AcceptConnection so that
// the entropy/logging hvsock setup succeeds. Returns the created controller.
func setupStartVMEnv(t *testing.T) (*Controller, *mocks.MockutilityVM, *mocks.MockguestManager) {
	t.Helper()

	c, uvm, guest := newControllerWithState(t, StateCreated)

	// listenHVSock always returns a fakeListener.
	swapListenHVSock(t, func(_ *winio.HvsockAddr) (net.Listener, error) {
		return &fakeListener{}, nil
	})

	// AcceptConnection returns a fakeConn (for entropy write and log relay).
	uvm.EXPECT().AcceptConnection(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&fakeConn{}, nil).AnyTimes()

	// RuntimeID is called by listenHVSock setup (for building the HvsockAddr).
	uvm.EXPECT().RuntimeID().Return(testGUID).AnyTimes()
	// ID is called by ParseGCSLogrus.
	uvm.EXPECT().ID().Return("test-vm-id").AnyTimes()

	return c, uvm, guest
}

func TestStartVM(t *testing.T) {
	ctx := context.Background()

	t.Run("prepare_connection_fails/state_invalid", func(t *testing.T) {
		c, uvm, guest := setupStartVMEnv(t)
		_ = uvm // AcceptConnection/RuntimeID already set up

		guest.EXPECT().PrepareConnection(gomock.Any()).Return(errors.New("prepare failed"))

		err := c.StartVM(ctx, &StartOptions{})
		if err == nil {
			t.Fatal("expected error when PrepareConnection fails")
		}
		if c.State() != StateInvalid {
			t.Errorf("expected StateInvalid, got %s", c.State())
		}
	})

	t.Run("uvm_start_fails/state_invalid", func(t *testing.T) {
		c, uvm, guest := setupStartVMEnv(t)

		guest.EXPECT().PrepareConnection(gomock.Any()).Return(nil)
		uvm.EXPECT().Start(gomock.Any()).Return(errors.New("start failed"))

		err := c.StartVM(ctx, &StartOptions{})
		if err == nil {
			t.Fatal("expected error when uvm.Start fails")
		}
		if c.State() != StateInvalid {
			t.Errorf("expected StateInvalid, got %s", c.State())
		}
	})

	t.Run("create_connection_fails/state_invalid", func(t *testing.T) {
		c, uvm, guest := setupStartVMEnv(t)

		guest.EXPECT().PrepareConnection(gomock.Any()).Return(nil)
		uvm.EXPECT().Start(gomock.Any()).Return(nil)
		uvm.EXPECT().Wait(gomock.Any()).Return(nil).AnyTimes()
		guest.EXPECT().CreateConnection(gomock.Any()).Return(errors.New("create conn failed"))

		err := c.StartVM(ctx, &StartOptions{})
		if err == nil {
			t.Fatal("expected error when CreateConnection fails")
		}
		if c.State() != StateInvalid {
			t.Errorf("expected StateInvalid, got %s", c.State())
		}
	})

	t.Run("add_security_policy_fails/state_invalid", func(t *testing.T) {
		// Covered by TestStartVM_AddSecurityPolicyFails in vm_createvm_lcow_test.go
		// (requires LCOW-specific sandbox options that can't be set portably).
		t.Skip("covered by LCOW-specific test file")
	})

	t.Run("full_success/state_running", func(t *testing.T) {
		// On WCOW, finalizeGCSConnection type-asserts c.guest to
		// *guestmanager.Guest (for UpdateHvSocketAddress, which is
		// WCOW-only). This panics with a mock. The LCOW path is a no-op.
		// WCOW's finalizeGCSConnection is tested at the Guest level.
		if !isLCOW() {
			t.Skip("WCOW finalizeGCSConnection requires concrete Guest (type assertion)")
		}
		c, uvm, guest := setupStartVMEnv(t)

		guest.EXPECT().PrepareConnection(gomock.Any()).Return(nil)
		uvm.EXPECT().Start(gomock.Any()).Return(nil)

		// Block the background waitForVMExit goroutine so it doesn't
		// race with the state assertion below. Without this, the mock
		// Wait returns immediately and the goroutine transitions state
		// to Terminated before we can check StateRunning.
		waitCh := make(chan struct{})
		t.Cleanup(func() { close(waitCh) })
		uvm.EXPECT().Wait(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
			<-waitCh
			return nil
		}).AnyTimes()

		guest.EXPECT().CreateConnection(gomock.Any()).Return(nil)

		err := c.StartVM(ctx, &StartOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.State() != StateRunning {
			t.Errorf("expected StateRunning, got %s", c.State())
		}
	})

	t.Run("entropy_listener_bind_fails/state_invalid", func(t *testing.T) {
		if !isLCOW() {
			t.Skip("WCOW setupEntropyListener is a no-op")
		}
		c, uvm, _ := newControllerWithState(t, StateCreated)
		uvm.EXPECT().RuntimeID().Return(testGUID).AnyTimes()

		// First listenHVSock call (entropy) fails.
		swapListenHVSock(t, func(_ *winio.HvsockAddr) (net.Listener, error) {
			return nil, errors.New("entropy bind failed")
		})

		err := c.StartVM(ctx, &StartOptions{})
		if err == nil {
			t.Fatal("expected error when entropy listener bind fails")
		}
		if c.State() != StateInvalid {
			t.Errorf("expected StateInvalid, got %s", c.State())
		}
	})

	t.Run("logging_listener_bind_fails/state_invalid", func(t *testing.T) {
		if !isLCOW() {
			t.Skip("WCOW setupLoggingListener uses a standalone goroutine, not errgroup")
		}
		c, uvm, _ := newControllerWithState(t, StateCreated)
		uvm.EXPECT().RuntimeID().Return(testGUID).AnyTimes()
		// The entropy goroutine is already dispatched on the errgroup
		// before the logging listener bind fails; provide AcceptConnection
		// for it so it doesn't panic.
		uvm.EXPECT().AcceptConnection(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&fakeConn{}, nil).AnyTimes()

		// First call (entropy) succeeds, second call (logging) fails.
		callCount := 0
		swapListenHVSock(t, func(_ *winio.HvsockAddr) (net.Listener, error) {
			callCount++
			if callCount == 1 {
				return &fakeListener{}, nil
			}
			return nil, errors.New("logging bind failed")
		})

		err := c.StartVM(ctx, &StartOptions{})
		if err == nil {
			t.Fatal("expected error when logging listener bind fails")
		}
		// logOutputDone should be closed on logging listener failure.
		select {
		case <-c.logOutputDone:
		default:
			t.Error("expected logOutputDone to be closed when logging listener fails")
		}
		if c.State() != StateInvalid {
			t.Errorf("expected StateInvalid, got %s", c.State())
		}
	})

	t.Run("entropy_accept_fails/errgroup_error", func(t *testing.T) {
		if !isLCOW() {
			t.Skip("WCOW setupEntropyListener is a no-op")
		}
		c, uvm, guest := newControllerWithState(t, StateCreated)
		uvm.EXPECT().RuntimeID().Return(testGUID).AnyTimes()
		uvm.EXPECT().ID().Return("test-vm-id").AnyTimes()

		swapListenHVSock(t, func(_ *winio.HvsockAddr) (net.Listener, error) {
			return &fakeListener{}, nil
		})
		// AcceptConnection fails (entropy accept error surfaces via errgroup).
		uvm.EXPECT().AcceptConnection(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("entropy accept failed")).AnyTimes()

		guest.EXPECT().PrepareConnection(gomock.Any()).Return(nil)
		uvm.EXPECT().Start(gomock.Any()).Return(nil)
		uvm.EXPECT().Wait(gomock.Any()).Return(nil).AnyTimes()

		err := c.StartVM(ctx, &StartOptions{})
		if err == nil {
			t.Fatal("expected error when entropy AcceptConnection fails")
		}
		if c.State() != StateInvalid {
			t.Errorf("expected StateInvalid, got %s", c.State())
		}
	})

	t.Run("logging_accept_fails/state_invalid", func(t *testing.T) {
		if !isLCOW() {
			t.Skip("WCOW setupLoggingListener uses a standalone goroutine, not errgroup")
		}
		c, uvm, guest := newControllerWithState(t, StateCreated)
		uvm.EXPECT().RuntimeID().Return(testGUID).AnyTimes()
		uvm.EXPECT().ID().Return("test-vm-id").AnyTimes()

		swapListenHVSock(t, func(_ *winio.HvsockAddr) (net.Listener, error) {
			return &fakeListener{}, nil
		})
		// First AcceptConnection (entropy) succeeds, second (logging) fails.
		acceptCount := 0
		uvm.EXPECT().AcceptConnection(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ net.Listener, _ bool) (net.Conn, error) {
				acceptCount++
				if acceptCount == 1 {
					return &fakeConn{}, nil
				}
				return nil, errors.New("logging accept failed")
			}).AnyTimes()

		guest.EXPECT().PrepareConnection(gomock.Any()).Return(nil)
		uvm.EXPECT().Start(gomock.Any()).Return(nil)
		uvm.EXPECT().Wait(gomock.Any()).Return(nil).AnyTimes()

		err := c.StartVM(ctx, &StartOptions{})
		if err == nil {
			t.Fatal("expected error when logging AcceptConnection fails")
		}
		if c.State() != StateInvalid {
			t.Errorf("expected StateInvalid, got %s", c.State())
		}
	})
}

// ─── 4. waitForVMExit Race ─────────────────────────────────────────────────────

func TestWaitForVMExit(t *testing.T) {
	ctx := context.Background()

	t.Run("natural_exit/sets_terminated", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)

		waitDone := make(chan struct{})
		uvm.EXPECT().Wait(gomock.Any()).DoAndReturn(func(context.Context) error {
			<-waitDone
			return nil
		})

		go c.waitForVMExit(ctx)

		// Unblock the Wait.
		close(waitDone)

		// Give the goroutine a moment to acquire the lock and set state.
		time.Sleep(50 * time.Millisecond)

		if c.State() != StateTerminated {
			t.Errorf("expected StateTerminated after natural exit, got %s", c.State())
		}
	})

	t.Run("concurrent_terminate/waitForVMExit_noops", func(t *testing.T) {
		c, uvm, guest := newControllerWithState(t, StateRunning)

		waitCh := make(chan struct{})
		uvm.EXPECT().Wait(gomock.Any()).DoAndReturn(func(context.Context) error {
			<-waitCh
			return nil
		}).AnyTimes()

		go c.waitForVMExit(ctx)

		// TerminateVM runs first and sets Terminated.
		uvm.EXPECT().Terminate(gomock.Any()).Return(nil)
		guest.EXPECT().CloseConnection().Return(nil)
		uvm.EXPECT().Close(gomock.Any()).Return(nil)

		if err := c.TerminateVM(ctx); err != nil {
			t.Fatalf("TerminateVM: %v", err)
		}

		// Now unblock waitForVMExit — it should see Terminated and no-op.
		close(waitCh)
		time.Sleep(50 * time.Millisecond)

		if c.State() != StateTerminated {
			t.Errorf("expected StateTerminated, got %s", c.State())
		}
	})
}

// ─── 5. ExecIntoHost ───────────────────────────────────────────────────────────

func TestExecIntoHost(t *testing.T) {
	ctx := context.Background()

	t.Run("terminal_with_stderr/returns_error", func(t *testing.T) {
		c, _, _ := newControllerWithState(t, StateRunning)
		_, err := c.ExecIntoHost(ctx, &shimdiag.ExecProcessRequest{
			Terminal: true,
			Stderr:   "/some/path",
		})
		if err == nil {
			t.Error("expected error when terminal=true and stderr is set")
		}
	})

	t.Run("success/returns_exit_code", func(t *testing.T) {
		c, _, guest := newControllerWithState(t, StateRunning)
		guest.EXPECT().ExecIntoUVM(gomock.Any(), gomock.Any()).Return(42, nil)

		exitCode, err := c.ExecIntoHost(ctx, &shimdiag.ExecProcessRequest{
			Args: []string{"echo", "hello"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exitCode != 42 {
			t.Errorf("expected exit code 42, got %d", exitCode)
		}
	})

	t.Run("guest_error/propagated", func(t *testing.T) {
		c, _, guest := newControllerWithState(t, StateRunning)
		guest.EXPECT().ExecIntoUVM(gomock.Any(), gomock.Any()).Return(-1, errors.New("exec failed"))

		_, err := c.ExecIntoHost(ctx, &shimdiag.ExecProcessRequest{
			Args: []string{"fail"},
		})
		if err == nil {
			t.Error("expected error from guest.ExecIntoUVM to be propagated")
		}
	})

	t.Run("active_exec_count_tracking", func(t *testing.T) {
		c, _, guest := newControllerWithState(t, StateRunning)

		// Block until we verify the count.
		execStarted := make(chan struct{})
		execContinue := make(chan struct{})
		guest.EXPECT().ExecIntoUVM(gomock.Any(), gomock.Any()).DoAndReturn(
			func(context.Context, *cmd.CmdProcessRequest) (int, error) {
				close(execStarted)
				<-execContinue
				return 0, nil
			},
		)

		go func() {
			_, _ = c.ExecIntoHost(ctx, &shimdiag.ExecProcessRequest{Args: []string{"sleep"}})
		}()

		<-execStarted
		if count := c.activeExecCount.Load(); count != 1 {
			t.Errorf("expected activeExecCount=1, got %d", count)
		}

		close(execContinue)
		time.Sleep(50 * time.Millisecond)

		if count := c.activeExecCount.Load(); count != 0 {
			t.Errorf("expected activeExecCount=0 after exec, got %d", count)
		}
	})
}

// ─── 6. DumpStacks ─────────────────────────────────────────────────────────────

func TestDumpStacks(t *testing.T) {
	ctx := context.Background()

	t.Run("supported/calls_guest", func(t *testing.T) {
		c, _, guest := newControllerWithState(t, StateRunning)
		guest.EXPECT().Capabilities().Return(stubCaps{dumpStacksSupported: true})
		guest.EXPECT().DumpStacks(gomock.Any()).Return("goroutine 1 [running]:\n...", nil)

		result, err := c.DumpStacks(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == "" {
			t.Error("expected non-empty stack dump")
		}
	})

	t.Run("unsupported/returns_empty", func(t *testing.T) {
		c, _, guest := newControllerWithState(t, StateRunning)
		guest.EXPECT().Capabilities().Return(stubCaps{dumpStacksSupported: false})
		// DumpStacks should NOT be called.

		result, err := c.DumpStacks(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "" {
			t.Errorf("expected empty result when unsupported, got: %q", result)
		}
	})

	t.Run("guest_error/propagated", func(t *testing.T) {
		c, _, guest := newControllerWithState(t, StateRunning)
		guest.EXPECT().Capabilities().Return(stubCaps{dumpStacksSupported: true})
		guest.EXPECT().DumpStacks(gomock.Any()).Return("", errors.New("dump failed"))

		_, err := c.DumpStacks(ctx)
		if err == nil {
			t.Error("expected error from guest.DumpStacks to be propagated")
		}
	})
}

// ─── 7. Wait ───────────────────────────────────────────────────────────────────

func TestWait(t *testing.T) {
	t.Run("not_created/returns_error", func(t *testing.T) {
		c := New()
		err := c.Wait(context.Background())
		if err == nil {
			t.Error("expected error for Wait on NotCreated controller")
		}
	})

	t.Run("vm_exits_normally", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		close(c.logOutputDone) // simulate log processing already done

		uvm.EXPECT().Wait(gomock.Any()).Return(nil)

		err := c.Wait(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("context_cancelled/joined_error", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		// logOutputDone never closes → ctx cancellation should produce an error.

		uvm.EXPECT().Wait(gomock.Any()).Return(nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		err := c.Wait(ctx)
		if err == nil {
			t.Error("expected error when context is cancelled and logOutputDone is not closed")
		}
	})

	t.Run("uvm_wait_error/propagated", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		close(c.logOutputDone)

		uvm.EXPECT().Wait(gomock.Any()).Return(errors.New("vm crashed"))

		err := c.Wait(context.Background())
		if err == nil {
			t.Error("expected error when uvm.Wait fails")
		}
	})

	t.Run("terminated_vm/immediate_return", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateTerminated)
		close(c.logOutputDone)

		uvm.EXPECT().Wait(gomock.Any()).Return(nil)

		err := c.Wait(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// ─── 8. Stats ──────────────────────────────────────────────────────────────────

func TestStats(t *testing.T) {
	ctx := context.Background()

	t.Run("not_running/returns_error", func(t *testing.T) {
		c, _, _ := newControllerWithState(t, StateCreated)
		_, err := c.Stats(ctx)
		if err == nil {
			t.Error("expected error for Stats on non-Running controller")
		}
	})

	t.Run("lookupVMMEM_fails/error_propagated", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		uvm.EXPECT().RuntimeID().Return(testGUID)

		swapLookupVMMEM(t, func(_ context.Context, _ guid.GUID, _ iwin.API) (windows.Handle, error) {
			return 0, errors.New("vmmem not found")
		})

		_, err := c.Stats(ctx)
		if err == nil {
			t.Error("expected error when lookupVMMEM fails")
		}
	})

	t.Run("propertiesV2_fails/error_propagated", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		uvm.EXPECT().RuntimeID().Return(testGUID)
		uvm.EXPECT().PropertiesV2(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("properties failed"))

		swapLookupVMMEM(t, func(_ context.Context, _ guid.GUID, _ iwin.API) (windows.Handle, error) {
			return windows.Handle(0x1234), nil
		})

		_, err := c.Stats(ctx)
		if err == nil {
			t.Error("expected error when PropertiesV2 fails")
		}
	})

	t.Run("va_backed/working_set_from_memcounters", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		c.isPhysicallyBacked = false

		uvm.EXPECT().RuntimeID().Return(testGUID)
		uvm.EXPECT().PropertiesV2(gomock.Any(), gomock.Any(), gomock.Any()).Return(&hcsschema.Properties{
			Statistics: &hcsschema.Statistics{
				Processor: &hcsschema.ProcessorStats{
					TotalRuntime100ns: 5000,
				},
			},
			Memory: &hcsschema.MemoryInformationForVm{
				VirtualMachineMemory: &hcsschema.VmMemory{
					AssignedMemory: 1024,
				},
			},
		}, nil)

		const fakeWSS uint = 8192
		swapLookupVMMEM(t, func(_ context.Context, _ guid.GUID, _ iwin.API) (windows.Handle, error) {
			return windows.Handle(0x1234), nil
		})
		swapGetProcessMemoryInfo(t, func(_ windows.Handle) (*process.ProcessMemoryCountersEx, error) {
			return &process.ProcessMemoryCountersEx{WorkingSetSize: fakeWSS}, nil
		})

		s, err := c.Stats(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.Memory == nil {
			t.Fatal("expected non-nil memory stats")
		}
		if s.Memory.WorkingSetBytes != uint64(fakeWSS) {
			t.Errorf("expected WorkingSetBytes=%d (from memCounters), got %d", fakeWSS, s.Memory.WorkingSetBytes)
		}
		if s.Processor == nil || s.Processor.TotalRuntimeNS != 500000 {
			t.Errorf("expected TotalRuntimeNS=500000, got %d", s.Processor.TotalRuntimeNS)
		}
	})

	t.Run("physically_backed/working_set_from_assigned_memory", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		c.isPhysicallyBacked = true

		uvm.EXPECT().RuntimeID().Return(testGUID)
		uvm.EXPECT().PropertiesV2(gomock.Any(), gomock.Any(), gomock.Any()).Return(&hcsschema.Properties{
			Statistics: &hcsschema.Statistics{
				Processor: &hcsschema.ProcessorStats{
					TotalRuntime100ns: 100,
				},
			},
			Memory: &hcsschema.MemoryInformationForVm{
				VirtualMachineMemory: &hcsschema.VmMemory{
					AssignedMemory: 256,
				},
			},
		}, nil)

		swapLookupVMMEM(t, func(_ context.Context, _ guid.GUID, _ iwin.API) (windows.Handle, error) {
			return windows.Handle(0x5678), nil
		})
		// getProcessMemoryInfo should NOT be called for physically-backed VMs.
		// If it is, the test will fail because we haven't set an expectation.

		s, err := c.Stats(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.Memory == nil {
			t.Fatal("expected non-nil memory stats")
		}
		// AssignedMemory * 4096
		expected := uint64(256 * 4096)
		if s.Memory.WorkingSetBytes != expected {
			t.Errorf("expected WorkingSetBytes=%d (AssignedMemory*4096), got %d", expected, s.Memory.WorkingSetBytes)
		}
	})

	t.Run("get_process_memory_info_fails/error_propagated", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		c.isPhysicallyBacked = false

		uvm.EXPECT().RuntimeID().Return(testGUID)
		uvm.EXPECT().PropertiesV2(gomock.Any(), gomock.Any(), gomock.Any()).Return(&hcsschema.Properties{
			Statistics: &hcsschema.Statistics{
				Processor: &hcsschema.ProcessorStats{},
			},
			Memory: &hcsschema.MemoryInformationForVm{
				VirtualMachineMemory: &hcsschema.VmMemory{},
			},
		}, nil)

		swapLookupVMMEM(t, func(_ context.Context, _ guid.GUID, _ iwin.API) (windows.Handle, error) {
			return windows.Handle(0x1234), nil
		})
		swapGetProcessMemoryInfo(t, func(_ windows.Handle) (*process.ProcessMemoryCountersEx, error) {
			return nil, errors.New("memory info failed")
		})

		_, err := c.Stats(ctx)
		if err == nil {
			t.Error("expected error when getProcessMemoryInfo fails")
		}
	})
}

// ─── 9. Accessor Methods ──────────────────────────────────────────────────────

func TestRuntimeID(t *testing.T) {
	t.Run("created/returns_guid_string", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateCreated)
		uvm.EXPECT().RuntimeID().Return(testGUID)

		rid := c.RuntimeID()
		if rid == "" {
			t.Error("expected non-empty RuntimeID for Created controller")
		}
	})

	t.Run("not_created/returns_empty", func(t *testing.T) {
		c := New()
		if rid := c.RuntimeID(); rid != "" {
			t.Errorf("expected empty RuntimeID for NotCreated controller, got %q", rid)
		}
	})
}

func TestStartTime(t *testing.T) {
	t.Run("running/returns_start_time", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		now := time.Now()
		uvm.EXPECT().StartedTime().Return(now)

		st := c.StartTime()
		if st != now {
			t.Errorf("expected start time %v, got %v", now, st)
		}
	})

	t.Run("not_created/returns_zero", func(t *testing.T) {
		c := New()
		st := c.StartTime()
		if !st.IsZero() {
			t.Errorf("expected zero start time for NotCreated, got %v", st)
		}
	})
}

func TestExitStatus(t *testing.T) {
	t.Run("terminated/returns_status", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateTerminated)
		now := time.Now()
		exitErr := errors.New("vm crashed")
		uvm.EXPECT().StoppedTime().Return(now)
		uvm.EXPECT().ExitError().Return(exitErr)

		es, err := c.ExitStatus()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if es.StoppedTime != now {
			t.Errorf("expected StoppedTime %v, got %v", now, es.StoppedTime)
		}
		if !errors.Is(es.Err, exitErr) {
			t.Errorf("expected ExitError %v, got %v", exitErr, es.Err)
		}
	})

	t.Run("running/returns_error", func(t *testing.T) {
		c, _, _ := newControllerWithState(t, StateRunning)
		_, err := c.ExitStatus()
		if err == nil {
			t.Error("expected error for ExitStatus on Running controller")
		}
	})
}

// ─── 10. UpdateCPUGroup ────────────────────────────────────────────────────────

func TestUpdateCPUGroup(t *testing.T) {
	ctx := context.Background()

	t.Run("empty_id/returns_error", func(t *testing.T) {
		c, _, _ := newControllerWithState(t, StateRunning)
		err := c.UpdateCPUGroup(ctx, "")
		if err == nil {
			t.Error("expected error when cpuGroupID is empty")
		}
	})

	t.Run("success", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		uvm.EXPECT().SetCPUGroup(gomock.Any(), gomock.Any()).Return(nil)

		err := c.UpdateCPUGroup(ctx, "group-123")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("set_cpu_group_fails/error_propagated", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		uvm.EXPECT().SetCPUGroup(gomock.Any(), gomock.Any()).Return(errors.New("set group failed"))

		err := c.UpdateCPUGroup(ctx, "group-123")
		if err == nil {
			t.Error("expected error when SetCPUGroup fails")
		}
	})
}

// ─── 12. UpdateCPU ─────────────────────────────────────────────────────────────

func TestUpdateCPU(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		uvm.EXPECT().UpdateCPULimits(gomock.Any(), gomock.Any()).Return(nil)

		err := c.UpdateCPU(ctx, &hcsschema.ProcessorLimits{})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("uvm_error/propagated", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		uvm.EXPECT().UpdateCPULimits(gomock.Any(), gomock.Any()).Return(errors.New("cpu limits failed"))

		err := c.UpdateCPU(ctx, &hcsschema.ProcessorLimits{})
		if err == nil {
			t.Error("expected error when UpdateCPULimits fails")
		}
	})
}

// ─── 13. UpdateMemory ──────────────────────────────────────────────────────────

func TestUpdateMemory(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		uvm.EXPECT().UpdateMemory(gomock.Any(), gomock.Any()).Return(nil)

		err := c.UpdateMemory(ctx, 1024)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("uvm_error/propagated", func(t *testing.T) {
		c, uvm, _ := newControllerWithState(t, StateRunning)
		uvm.EXPECT().UpdateMemory(gomock.Any(), gomock.Any()).Return(errors.New("memory update failed"))

		err := c.UpdateMemory(ctx, 1024)
		if err == nil {
			t.Error("expected error when UpdateMemory fails")
		}
	})
}

// ─── 14. UpdatePolicyFragment ──────────────────────────────────────────────────

func TestUpdatePolicyFragment(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		c, _, guest := newControllerWithState(t, StateRunning)
		guest.EXPECT().InjectPolicyFragment(gomock.Any(), gomock.Any()).Return(nil)

		err := c.UpdatePolicyFragment(ctx, guestresource.SecurityPolicyFragment{})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("guest_error/propagated", func(t *testing.T) {
		c, _, guest := newControllerWithState(t, StateRunning)
		guest.EXPECT().InjectPolicyFragment(gomock.Any(), gomock.Any()).Return(errors.New("inject failed"))

		err := c.UpdatePolicyFragment(ctx, guestresource.SecurityPolicyFragment{})
		if err == nil {
			t.Error("expected error when InjectPolicyFragment fails")
		}
	})
}

// ─── 15. Concurrent access sanity ──────────────────────────────────────────────

func TestConcurrentStateAccess(t *testing.T) {
	c, uvm, guest := newControllerWithState(t, StateRunning)
	ctx := context.Background()

	// Allow TerminateVM to be called from one goroutine.
	uvm.EXPECT().Terminate(gomock.Any()).Return(nil).AnyTimes()
	guest.EXPECT().CloseConnection().Return(nil).AnyTimes()
	uvm.EXPECT().Close(gomock.Any()).Return(nil).AnyTimes()

	var wg sync.WaitGroup
	// Read state concurrently while another goroutine terminates.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.State()
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = c.TerminateVM(ctx)
	}()

	wg.Wait()
	// No race detector panic = pass.
}
