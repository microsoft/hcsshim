//go:build windows && (lcow || wcow)

package vm

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/Microsoft/hcsshim/internal/controller/vm/mocks"
	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
)

var (
	errUVMTerminate = errors.New("uvm terminate failed")
	errUVMClose     = errors.New("uvm close failed")
	errGuestClose   = errors.New("guest close failed")
	errGuestExec    = errors.New("guest exec failed")
	errGuestDump    = errors.New("guest dump failed")
	errGuestPolicy  = errors.New("guest policy failed")
)

type stubCaps struct {
	dumpStacks bool
}

func (s stubCaps) IsSignalProcessSupported() bool        { return false }
func (s stubCaps) IsDeleteContainerStateSupported() bool { return false }
func (s stubCaps) IsDumpStacksSupported() bool           { return s.dumpStacks }
func (s stubCaps) IsNamespaceAddRequestSupported() bool  { return false }

var _ gcs.GuestDefinedCapabilities = stubCaps{}

func newControllerWithState(t *testing.T, state State) (*Controller, *mocks.MockvmLifetime, *mocks.MockguestManager) {
	t.Helper()
	ctrl := gomock.NewController(t)
	uvm := mocks.NewMockvmLifetime(ctrl)
	guest := mocks.NewMockguestManager(ctrl)
	c := &Controller{
		uvm:           uvm,
		guest:         guest,
		vmState:       state,
		logOutputDone: make(chan struct{}),
	}
	return c, uvm, guest
}

// ─────────────────────────────────────────────────────────────────────────────
// Idempotency
// ─────────────────────────────────────────────────────────────────────────────

func TestStartVM_AlreadyRunning_NoOp(t *testing.T) {
	c, _, _ := newControllerWithState(t, StateRunning)

	if err := c.StartVM(context.Background(), &StartOptions{}); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := c.State(); got != StateRunning {
		t.Errorf("expected state to remain Running, got %s", got)
	}
}

func TestTerminateVM_FromTerminated_NoOp(t *testing.T) {
	c, _, _ := newControllerWithState(t, StateTerminated)

	if err := c.TerminateVM(context.Background()); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestTerminateVM_FromNotCreated_NoOp(t *testing.T) {
	c, _, _ := newControllerWithState(t, StateNotCreated)

	if err := c.TerminateVM(context.Background()); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// State guards
// ─────────────────────────────────────────────────────────────────────────────

func TestStartVM_RejectsWrongState(t *testing.T) {
	for _, tt := range []struct {
		name  string
		state State
	}{
		{"NotCreated", StateNotCreated},
		{"Terminated", StateTerminated},
		{"Invalid", StateInvalid},
	} {
		t.Run("reject/"+tt.name, func(t *testing.T) {
			c, _, _ := newControllerWithState(t, tt.state)
			if err := c.StartVM(context.Background(), &StartOptions{}); err == nil {
				t.Fatalf("expected error for StartVM on %s, got nil", tt.state)
			}
		})
	}
}

func TestExecIntoHost_RejectsTerminated(t *testing.T) {
	c, _, _ := newControllerWithState(t, StateTerminated)

	code, err := c.ExecIntoHost(context.Background(), &shimdiag.ExecProcessRequest{Args: []string{"true"}})
	if err == nil {
		t.Fatal("expected error for exec on terminated VM, got nil")
	}
	if code != -1 {
		t.Errorf("expected exit code -1, got %d", code)
	}
}

func TestExecIntoHost_TerminalAndStderr_RejectsConfig(t *testing.T) {
	c, _, _ := newControllerWithState(t, StateRunning)

	code, err := c.ExecIntoHost(context.Background(), &shimdiag.ExecProcessRequest{
		Terminal: true,
		Stderr:   "stderr.log",
	})
	if err == nil {
		t.Fatal("expected error for terminal+stderr, got nil")
	}
	if code != -1 {
		t.Errorf("expected exit code -1, got %d", code)
	}
}

func TestUpdateMethods_RejectNonRunning(t *testing.T) {
	for _, tt := range []struct {
		name string
		call func(c *Controller) error
	}{
		{"UpdateCPU", func(c *Controller) error { return c.UpdateCPU(context.Background(), nil) }},
		{"UpdateMemory", func(c *Controller) error { return c.UpdateMemory(context.Background(), 1024) }},
		{"UpdateCPUGroup", func(c *Controller) error { return c.UpdateCPUGroup(context.Background(), "id") }},
		{"UpdatePolicyFragment", func(c *Controller) error {
			return c.UpdatePolicyFragment(context.Background(), guestresource.SecurityPolicyFragment{})
		}},
	} {
		t.Run("reject/"+tt.name, func(t *testing.T) {
			c, _, _ := newControllerWithState(t, StateCreated)
			if err := tt.call(c); err == nil {
				t.Fatal("expected error for update on non-Running VM, got nil")
			}
		})
	}
}

func TestStats_RejectsTerminated(t *testing.T) {
	c, _, _ := newControllerWithState(t, StateTerminated)

	if _, err := c.Stats(context.Background()); err == nil {
		t.Fatal("expected error for Stats on terminated VM, got nil")
	}
}

func TestWait_RejectsNotCreated(t *testing.T) {
	c, _, _ := newControllerWithState(t, StateNotCreated)

	if err := c.Wait(context.Background()); err == nil {
		t.Fatal("expected error for Wait on NotCreated VM, got nil")
	}
}

func TestDumpStacks_RejectsTerminated(t *testing.T) {
	c, _, _ := newControllerWithState(t, StateTerminated)

	if _, err := c.DumpStacks(context.Background()); err == nil {
		t.Fatal("expected error for DumpStacks on terminated VM, got nil")
	}
}

func TestUpdateCPUGroup_EmptyID_RejectsBeforeUVM(t *testing.T) {
	c, _, _ := newControllerWithState(t, StateRunning)

	if err := c.UpdateCPUGroup(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty cpuGroupID, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TerminateVM cleanup chain
// ─────────────────────────────────────────────────────────────────────────────

func TestTerminateVM_FromCreated_DrivesFullCleanup(t *testing.T) {
	c, uvm, guest := newControllerWithState(t, StateCreated)
	gomock.InOrder(
		uvm.EXPECT().Terminate(gomock.Any()).Return(nil),
		guest.EXPECT().CloseConnection().Return(nil),
		uvm.EXPECT().Close(gomock.Any()).Return(nil),
	)

	if err := c.TerminateVM(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := c.State(); got != StateTerminated {
		t.Errorf("expected Terminated, got %s", got)
	}
}

func TestTerminateVM_GuestCloseFails_StillCallsUVMClose(t *testing.T) {
	c, uvm, guest := newControllerWithState(t, StateRunning)
	gomock.InOrder(
		uvm.EXPECT().Terminate(gomock.Any()).Return(nil),
		guest.EXPECT().CloseConnection().Return(errGuestClose),
		uvm.EXPECT().Close(gomock.Any()).Return(nil),
	)

	if err := c.TerminateVM(context.Background()); err != nil {
		t.Fatalf("guest close failure must be swallowed, got %v", err)
	}
}

func TestTerminateVM_UVMTerminateFails_StillCallsCloseConnectionAndClose(t *testing.T) {
	c, uvm, guest := newControllerWithState(t, StateRunning)
	gomock.InOrder(
		uvm.EXPECT().Terminate(gomock.Any()).Return(errUVMTerminate),
		guest.EXPECT().CloseConnection().Return(nil),
		uvm.EXPECT().Close(gomock.Any()).Return(nil),
	)

	if err := c.TerminateVM(context.Background()); err != nil {
		t.Fatalf("uvm.Terminate failure must be swallowed, got %v", err)
	}
}

func TestTerminateVM_UVMCloseFails_TransitionsToInvalid(t *testing.T) {
	c, uvm, guest := newControllerWithState(t, StateRunning)
	gomock.InOrder(
		uvm.EXPECT().Terminate(gomock.Any()).Return(nil),
		guest.EXPECT().CloseConnection().Return(nil),
		uvm.EXPECT().Close(gomock.Any()).Return(errUVMClose),
	)

	err := c.TerminateVM(context.Background())
	if !errors.Is(err, errUVMClose) {
		t.Errorf("expected wrapped errUVMClose, got %v", err)
	}
	if got := c.State(); got != StateInvalid {
		t.Errorf("expected Invalid, got %s", got)
	}
}

func TestTerminateVM_FromInvalid_RecoversToTerminated(t *testing.T) {
	c, uvm, guest := newControllerWithState(t, StateInvalid)
	gomock.InOrder(
		uvm.EXPECT().Terminate(gomock.Any()).Return(nil),
		guest.EXPECT().CloseConnection().Return(nil),
		uvm.EXPECT().Close(gomock.Any()).Return(nil),
	)

	if err := c.TerminateVM(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := c.State(); got != StateTerminated {
		t.Errorf("expected Terminated, got %s", got)
	}
}

func TestTerminateVM_FromInvalid_StaysInvalidOnRetryFailure(t *testing.T) {
	c, uvm, guest := newControllerWithState(t, StateInvalid)
	uvm.EXPECT().Terminate(gomock.Any()).Return(nil)
	guest.EXPECT().CloseConnection().Return(nil)
	uvm.EXPECT().Close(gomock.Any()).Return(errUVMClose)

	err := c.TerminateVM(context.Background())
	if !errors.Is(err, errUVMClose) {
		t.Fatalf("expected errUVMClose, got %v", err)
	}
	if got := c.State(); got != StateInvalid {
		t.Errorf("expected Invalid preserved, got %s", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ExitStatus
// ─────────────────────────────────────────────────────────────────────────────

func TestExitStatus_NotTerminated_ReturnsError(t *testing.T) {
	c, _, _ := newControllerWithState(t, StateRunning)

	if _, err := c.ExitStatus(); err == nil {
		t.Fatal("expected error for non-Terminated VM, got nil")
	}
}

func TestExitStatus_Terminated_ReturnsValue(t *testing.T) {
	c, uvm, _ := newControllerWithState(t, StateTerminated)
	wantErr := errors.New("vm crashed")
	uvm.EXPECT().StoppedTime().Return(time.Now())
	uvm.EXPECT().ExitError().Return(wantErr)

	st, err := c.ExitStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !errors.Is(st.Err, wantErr) {
		t.Errorf("expected ExitError %v, got %v", wantErr, st.Err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ExecIntoHost delegation
// ─────────────────────────────────────────────────────────────────────────────

func TestExecIntoHost_Running_DelegatesToGuest(t *testing.T) {
	c, _, guest := newControllerWithState(t, StateRunning)
	guest.EXPECT().ExecIntoUVM(gomock.Any(), gomock.Any()).Return(42, nil)

	code, err := c.ExecIntoHost(context.Background(), &shimdiag.ExecProcessRequest{Args: []string{"true"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 42 {
		t.Errorf("expected exit code 42, got %d", code)
	}
}

func TestExecIntoHost_GuestError_PropagatesUnchanged(t *testing.T) {
	c, _, guest := newControllerWithState(t, StateRunning)
	guest.EXPECT().ExecIntoUVM(gomock.Any(), gomock.Any()).Return(-1, errGuestExec)

	code, err := c.ExecIntoHost(context.Background(), &shimdiag.ExecProcessRequest{Args: []string{"false"}})
	if !errors.Is(err, errGuestExec) {
		t.Fatalf("expected errGuestExec, got %v", err)
	}
	if code != -1 {
		t.Errorf("expected exit code -1, got %d", code)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// DumpStacks
// ─────────────────────────────────────────────────────────────────────────────

func TestDumpStacks_NoCapability_ReturnsEmpty(t *testing.T) {
	c, _, guest := newControllerWithState(t, StateRunning)
	guest.EXPECT().Capabilities().Return(stubCaps{dumpStacks: false})

	out, err := c.DumpStacks(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty output, got %q", out)
	}
}

func TestDumpStacks_HasCapability_DelegatesToGuest(t *testing.T) {
	c, _, guest := newControllerWithState(t, StateRunning)
	guest.EXPECT().Capabilities().Return(stubCaps{dumpStacks: true})
	guest.EXPECT().DumpStacks(gomock.Any()).Return("goroutine 1 [running]:\n", nil)

	out, err := c.DumpStacks(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestDumpStacks_GuestError_PropagatesUnchanged(t *testing.T) {
	c, _, guest := newControllerWithState(t, StateRunning)
	guest.EXPECT().Capabilities().Return(stubCaps{dumpStacks: true})
	guest.EXPECT().DumpStacks(gomock.Any()).Return("", errGuestDump)

	_, err := c.DumpStacks(context.Background())
	if !errors.Is(err, errGuestDump) {
		t.Fatalf("expected errGuestDump, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// UpdatePolicyFragment
// ─────────────────────────────────────────────────────────────────────────────

func TestUpdatePolicyFragment_Running_DelegatesToGuest(t *testing.T) {
	c, _, guest := newControllerWithState(t, StateRunning)
	guest.EXPECT().InjectPolicyFragment(gomock.Any(), gomock.Any()).Return(errGuestPolicy)

	err := c.UpdatePolicyFragment(context.Background(), guestresource.SecurityPolicyFragment{})
	if !errors.Is(err, errGuestPolicy) {
		t.Errorf("expected errGuestPolicy, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Wait + background goroutine
// ─────────────────────────────────────────────────────────────────────────────

func TestWait_Terminated_ReturnsAfterDrainingLogOutput(t *testing.T) {
	c, uvm, _ := newControllerWithState(t, StateTerminated)
	uvm.EXPECT().Wait(gomock.Any()).Return(nil)
	close(c.logOutputDone)

	if err := c.Wait(context.Background()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWait_ContextCancelledDuringLogDrain(t *testing.T) {
	c, uvm, _ := newControllerWithState(t, StateRunning)
	uvm.EXPECT().Wait(gomock.Any()).Return(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Wait(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected wrapped context.Canceled, got %v", err)
	}
}

func TestWait_Running_HappyPath(t *testing.T) {
	c, uvm, _ := newControllerWithState(t, StateRunning)
	uvm.EXPECT().Wait(gomock.Any()).Return(nil)
	close(c.logOutputDone)

	if err := c.Wait(context.Background()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWaitForVMExit_TransitionsRunningToTerminated(t *testing.T) {
	c, uvm, _ := newControllerWithState(t, StateRunning)
	uvm.EXPECT().Wait(gomock.Any()).Return(nil)

	c.waitForVMExit(context.Background())

	if got := c.State(); got != StateTerminated {
		t.Errorf("expected Terminated, got %s", got)
	}
}

func TestWaitForVMExit_OverwritesInvalidToTerminated(t *testing.T) {
	c, uvm, _ := newControllerWithState(t, StateInvalid)
	uvm.EXPECT().Wait(gomock.Any()).Return(nil)

	c.waitForVMExit(context.Background())

	if got := c.State(); got != StateTerminated {
		t.Errorf("expected Terminated (current behaviour overwrites Invalid), got %s", got)
	}
}
