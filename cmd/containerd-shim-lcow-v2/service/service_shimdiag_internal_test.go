//go:build windows && lcow

package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/Microsoft/hcsshim/internal/controller/vm"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
)

// Sentinel errors used by the shimdiag tests.
var (
	errExecHost  = errors.New("exec into host failed")
	errDumpStack = errors.New("dump stacks failed")
)

// ─── DiagPid tests ────────────────────────────────────────────────────────

// TestDiagPid_ReturnsCurrentPid verifies that DiagPid returns the calling
// process PID. This is the simplest diagnostic and a regression here would
// silently break shim discovery in operator tooling.
func TestDiagPid_ReturnsCurrentPid(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)

	resp, err := svc.DiagPid(context.Background(), &shimdiag.PidRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := resp.Pid, int32(os.Getpid()); got != want {
		t.Errorf("Pid = %d, want %d", got, want)
	}
}

// ─── diagExecInHostInternal tests ─────────────────────────────────────────

// TestDiagExecInHost_Success verifies happy-path delegation to
// vmController.ExecIntoHost: the request is forwarded unchanged and the
// exit code is plumbed back into the response.
func TestDiagExecInHost_Success(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)

	req := &shimdiag.ExecProcessRequest{Args: []string{"/bin/ls"}, Workdir: "/"}
	mockCtrl.EXPECT().ExecIntoHost(gomock.Any(), req).Return(42, nil)

	resp, err := svc.diagExecInHostInternal(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := resp.ExitCode, int32(42); got != want {
		t.Errorf("ExitCode = %d, want %d", got, want)
	}
}

// TestDiagExecInHost_Failure verifies that ExecIntoHost errors are wrapped
// before being returned through the gRPC layer.
func TestDiagExecInHost_Failure(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)

	req := &shimdiag.ExecProcessRequest{Args: []string{"/bin/false"}}
	mockCtrl.EXPECT().ExecIntoHost(gomock.Any(), req).Return(0, errExecHost)

	_, err := svc.diagExecInHostInternal(context.Background(), req)
	if err == nil {
		t.Fatal("expected error from ExecIntoHost, got nil")
	}
	if !errors.Is(err, errExecHost) {
		t.Errorf("expected error to wrap errExecHost, got %v", err)
	}
}

// ─── diagTasksInternal tests ──────────────────────────────────────────────

// TestDiagTasks_RejectVMNotRunning verifies that diagTasksInternal refuses
// to enumerate while the VM is not running; before VM start there are no
// real containers and a regression here would surface stale or empty data.
func TestDiagTasks_RejectVMNotRunning(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	mockCtrl.EXPECT().State().Return(vm.StateNotCreated).AnyTimes()

	_, err := svc.diagTasksInternal(context.Background(), &shimdiag.TasksRequest{})
	if err == nil {
		t.Fatal("expected error for VM not running, got nil")
	}
	if !strings.Contains(err.Error(), "vm is not running") {
		t.Errorf("expected error to contain %q, got %q", "vm is not running", err.Error())
	}
}

// TestDiagTasks_EmptyWhenNoPods verifies that a Running VM with no
// registered pods returns an empty task list rather than an error.
func TestDiagTasks_EmptyWhenNoPods(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	mockCtrl.EXPECT().State().Return(vm.StateRunning)

	resp, err := svc.diagTasksInternal(context.Background(), &shimdiag.TasksRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(resp.Tasks))
	}
}

// ─── diagShareInternal tests ──────────────────────────────────────────────

// TestDiagShare_RejectVMNotRunning verifies the VM-state guard for share
// requests; without it the plan9 controller call would dereference the
// underlying nil VM pointer.
func TestDiagShare_RejectVMNotRunning(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	mockCtrl.EXPECT().State().Return(vm.StateNotCreated).AnyTimes()

	_, err := svc.diagShareInternal(context.Background(), &shimdiag.ShareRequest{HostPath: "C:\\nonexistent"})
	if err == nil {
		t.Fatal("expected error for VM not running, got nil")
	}
	if !strings.Contains(err.Error(), "vm is not running") {
		t.Errorf("expected error to contain %q, got %q", "vm is not running", err.Error())
	}
}

// TestDiagShare_RejectMissingHostPath verifies that an unstattable host path
// is rejected before the plan9 reservation begins; this ensures the caller
// gets a clear error instead of a misleading reservation failure.
func TestDiagShare_RejectMissingHostPath(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	mockCtrl.EXPECT().State().Return(vm.StateRunning)

	missing := "Q:\\does-not-exist-" + t.Name()
	_, err := svc.diagShareInternal(context.Background(), &shimdiag.ShareRequest{HostPath: missing})
	if err == nil {
		t.Fatal("expected error for missing host path, got nil")
	}
	if !strings.Contains(err.Error(), "failed to open source path") {
		t.Errorf("expected error to contain %q, got %q", "failed to open source path", err.Error())
	}
}

// ─── diagStacksInternal tests ─────────────────────────────────────────────

// TestDiagStacks_RejectVMNotRunning verifies the VM-state guard;
// diagStacksInternal must not attempt to dump guest stacks before the GCS
// connection is up.
func TestDiagStacks_RejectVMNotRunning(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	mockCtrl.EXPECT().State().Return(vm.StateNotCreated).AnyTimes()

	_, err := svc.diagStacksInternal(context.Background())
	if err == nil {
		t.Fatal("expected error for VM not running, got nil")
	}
	if !strings.Contains(err.Error(), "vm is not running") {
		t.Errorf("expected error to contain %q, got %q", "vm is not running", err.Error())
	}
}

// TestDiagStacks_DumpStacksFailure verifies that a guest-side dump failure
// is wrapped and returned; without this the operator would lose the cause
// when triaging a hung shim.
func TestDiagStacks_DumpStacksFailure(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	mockCtrl.EXPECT().State().Return(vm.StateRunning)
	mockCtrl.EXPECT().DumpStacks(gomock.Any()).Return("", errDumpStack)

	_, err := svc.diagStacksInternal(context.Background())
	if err == nil {
		t.Fatal("expected error from DumpStacks, got nil")
	}
	if !errors.Is(err, errDumpStack) {
		t.Errorf("expected error to wrap errDumpStack, got %v", err)
	}
}

// TestDiagStacks_Success verifies that the response contains both the host
// stack dump (always populated by runtime.Stack) and the guest dump returned
// by the mock. A regression that drops either field would silently degrade
// debug output.
func TestDiagStacks_Success(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	mockCtrl.EXPECT().State().Return(vm.StateRunning)
	mockCtrl.EXPECT().DumpStacks(gomock.Any()).Return("guest-goroutine-1", nil)

	resp, err := svc.diagStacksInternal(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Stacks == "" {
		t.Error("expected host Stacks to be populated")
	}
	if resp.GuestStacks != "guest-goroutine-1" {
		t.Errorf("GuestStacks = %q, want %q", resp.GuestStacks, "guest-goroutine-1")
	}
}
