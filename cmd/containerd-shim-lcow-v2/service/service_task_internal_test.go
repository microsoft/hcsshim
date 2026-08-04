//go:build windows && lcow

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"

	"github.com/Microsoft/hcsshim/cmd/containerd-shim-lcow-v2/service/mocks"
	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	"github.com/Microsoft/hcsshim/internal/controller/vm"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/ctrdtaskapi"

	task "github.com/containerd/containerd/api/runtime/task/v3"
	containerdtypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/errdefs"
	"github.com/containerd/typeurl/v2"
)

// Sentinel errors used by the task tests to assert that the service wraps and
// propagates errors from the underlying vm controller.
var (
	errVMUpdatePolicy   = errors.New("vm update policy failed")
	errVMUpdateMemory   = errors.New("vm update memory failed")
	errVMUpdateCPU      = errors.New("vm update cpu failed")
	errVMUpdateCPUGroup = errors.New("vm update cpu group failed")
)

// ─── ensureVMRunning guard ────────────────────────────────────────────────

// TestTaskMethods_RejectVMNotRunning verifies that task internal methods which
// require a booted VM enforce the VM-must-be-running precondition. We exercise
// one representative not-running state (NotCreated); a regression in the guard
// would let containerd issue task RPCs against a VM that has not booted.
//
// state, delete, kill, and wait are intentionally excluded: they omit the
// VM-running guard so they can run during task teardown / migration aborts, so
// they operate on container bookkeeping and surface NotFound when the container
// is absent (see TestTaskMethods_RejectUnknownContainer) rather than a
// precondition error.
func TestTaskMethods_RejectVMNotRunning(t *testing.T) {
	tests := []struct {
		name string
		call func(*Service) error
	}{
		{
			name: "createInternal",
			call: func(svc *Service) error {
				_, err := svc.createInternal(context.Background(), &task.CreateTaskRequest{ID: "ctr", Bundle: t.TempDir()})
				return err
			},
		},
		{
			name: "startInternal",
			call: func(svc *Service) error {
				_, err := svc.startInternal(context.Background(), &task.StartRequest{ID: "ctr"})
				return err
			},
		},
		{
			name: "pidsInternal",
			call: func(svc *Service) error {
				_, err := svc.pidsInternal(context.Background(), &task.PidsRequest{ID: "ctr"})
				return err
			},
		},
		{
			name: "execInternal",
			call: func(svc *Service) error {
				_, err := svc.execInternal(context.Background(), &task.ExecProcessRequest{ID: "ctr"})
				return err
			},
		},
		{
			name: "resizePtyInternal",
			call: func(svc *Service) error {
				_, err := svc.resizePtyInternal(context.Background(), &task.ResizePtyRequest{ID: "ctr"})
				return err
			},
		},
		{
			name: "closeIOInternal",
			call: func(svc *Service) error {
				_, err := svc.closeIOInternal(context.Background(), &task.CloseIORequest{ID: "ctr"})
				return err
			},
		},
		{
			name: "updateInternal",
			call: func(svc *Service) error {
				_, err := svc.updateInternal(context.Background(), &task.UpdateTaskRequest{ID: "ctr"})
				return err
			},
		},
		{
			name: "statsInternal",
			call: func(svc *Service) error {
				_, err := svc.statsInternal(context.Background(), &task.StatsRequest{ID: "ctr"})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run("reject/"+tt.name, func(t *testing.T) {
			t.Parallel()
			svc, mockCtrl := newTestService(t)
			mockCtrl.EXPECT().State().Return(vm.StateNotCreated).AnyTimes()

			err := tt.call(svc)
			if err == nil {
				t.Fatal("expected error for VM not running, got nil")
			}
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Errorf("expected error to wrap ErrFailedPrecondition, got %v", err)
			}
		})
	}
}

// ─── Container lookup tests ───────────────────────────────────────────────

// TestTaskMethods_RejectUnknownContainer verifies that methods which need a
// container controller surface a NotFound when the container ID is not
// registered. A regression here would lead to a nil-deref deeper in the
// per-container code paths.
func TestTaskMethods_RejectUnknownContainer(t *testing.T) {
	tests := []struct {
		name string
		call func(*Service) error
	}{
		{
			name: "stateInternal",
			call: func(svc *Service) error {
				_, err := svc.stateInternal(context.Background(), &task.StateRequest{ID: "missing-ctr"})
				return err
			},
		},
		{
			name: "deleteInternal",
			call: func(svc *Service) error {
				_, err := svc.deleteInternal(context.Background(), &task.DeleteRequest{ID: "missing-ctr"})
				return err
			},
		},
		{
			name: "pidsInternal",
			call: func(svc *Service) error {
				_, err := svc.pidsInternal(context.Background(), &task.PidsRequest{ID: "missing-ctr"})
				return err
			},
		},
		{
			name: "killInternal",
			call: func(svc *Service) error {
				_, err := svc.killInternal(context.Background(), &task.KillRequest{ID: "missing-ctr"})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run("notfound/"+tt.name, func(t *testing.T) {
			t.Parallel()
			svc, mockCtrl := newTestService(t)
			mockCtrl.EXPECT().State().Return(vm.StateRunning).AnyTimes()

			err := tt.call(svc)
			if err == nil {
				t.Fatal("expected error for unknown container, got nil")
			}
			if !errors.Is(err, errdefs.ErrNotFound) {
				t.Errorf("expected error to wrap ErrNotFound, got %v", err)
			}
		})
	}
}

// ─── Not-implemented stubs ────────────────────────────────────────────────

// TestTaskMethods_NotImplemented verifies that the methods this shim does
// not implement return errdefs.ErrNotImplemented; containerd uses this to
// detect optional capabilities.
func TestTaskMethods_NotImplemented(t *testing.T) {
	tests := []struct {
		name string
		call func(*Service) error
	}{
		{
			name: "pauseInternal",
			call: func(svc *Service) error {
				_, err := svc.pauseInternal(context.Background(), &task.PauseRequest{ID: "ctr"})
				return err
			},
		},
		{
			name: "resumeInternal",
			call: func(svc *Service) error {
				_, err := svc.resumeInternal(context.Background(), &task.ResumeRequest{ID: "ctr"})
				return err
			},
		},
		{
			name: "checkpointInternal",
			call: func(svc *Service) error {
				_, err := svc.checkpointInternal(context.Background(), &task.CheckpointTaskRequest{ID: "ctr"})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run("unimplemented/"+tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _ := newTestService(t)

			err := tt.call(svc)
			if !errors.Is(err, errdefs.ErrNotImplemented) {
				t.Errorf("expected ErrNotImplemented, got %v", err)
			}
		})
	}
}

// TestShutdown_NoOp verifies that shutdownInternal is a no-op for this shim;
// the real teardown is driven by SandboxService.ShutdownSandbox and a
// regression here would terminate the shim prematurely.
func TestShutdown_NoOp(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)

	resp, err := svc.shutdownInternal(context.Background(), &task.ShutdownRequest{ID: "ctr"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// ─── Update dispatch tests ────────────────────────────────────────────────

// TestUpdate_NilResourcesRejected verifies that a nil Resources field is
// rejected before reaching typeurl.UnmarshalAny; without the guard, the
// unmarshal call would panic on the nil dereference.
func TestUpdate_NilResourcesRejected(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	mockCtrl.EXPECT().State().Return(vm.StateRunning)

	_, err := svc.updateInternal(context.Background(), &task.UpdateTaskRequest{ID: "ctr"})
	if err == nil {
		t.Fatal("expected error for nil Resources, got nil")
	}
	if !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Errorf("expected error to wrap ErrInvalidArgument, got %v", err)
	}
}

// TestUpdate_PolicyFragmentDispatch verifies the pod-level update path for a
// security-policy-fragment payload: the resource is unmarshalled via typeurl
// and forwarded to vmController.UpdatePolicyFragment with the same fragment.
func TestUpdate_PolicyFragmentDispatch(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.podControllers["pod-1"] = nil // sentinel: pod is known, no real controller needed

	any, err := typeurl.MarshalAnyToProto(&ctrdtaskapi.PolicyFragment{Fragment: "test-fragment"})
	if err != nil {
		t.Fatalf("marshal fragment: %v", err)
	}

	mockCtrl.EXPECT().State().Return(vm.StateRunning)
	mockCtrl.EXPECT().
		UpdatePolicyFragment(gomock.Any(), guestresource.SecurityPolicyFragment{Fragment: "test-fragment"}).
		Return(nil)

	if _, err := svc.updateInternal(context.Background(), &task.UpdateTaskRequest{
		ID:        "pod-1",
		Resources: any,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestUpdate_MemoryDispatch verifies that a LinuxResources update with a
// memory limit is converted to MiB and forwarded to vmController.UpdateMemory.
// The conversion is critical: a regression that forgets the divide would
// request gigabyte-scale memory in MiB and trigger HCS validation failures.
func TestUpdate_MemoryDispatch(t *testing.T) {
	t.Parallel()

	const memoryBytes = int64(2 * 1024 * 1024 * 1024) // 2 GiB
	const wantMiB = uint64(2 * 1024)

	svc, mockCtrl := newTestService(t)
	svc.podControllers["pod-1"] = nil

	limit := memoryBytes
	any, err := typeurl.MarshalAnyToProto(&specs.LinuxResources{
		Memory: &specs.LinuxMemory{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("marshal resources: %v", err)
	}

	mockCtrl.EXPECT().State().Return(vm.StateRunning)
	mockCtrl.EXPECT().UpdateMemory(gomock.Any(), wantMiB).Return(nil)

	if _, err := svc.updateInternal(context.Background(), &task.UpdateTaskRequest{
		ID:        "pod-1",
		Resources: any,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestUpdate_CPUDispatch verifies that a LinuxResources update with CPU
// quota+shares is mapped to ProcessorLimits{Limit, Weight} and forwarded.
func TestUpdate_CPUDispatch(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.podControllers["pod-1"] = nil

	quota := int64(50000)
	shares := uint64(1024)
	any, err := typeurl.MarshalAnyToProto(&specs.LinuxResources{
		CPU: &specs.LinuxCPU{Quota: &quota, Shares: &shares},
	})
	if err != nil {
		t.Fatalf("marshal resources: %v", err)
	}

	wantLimits := &hcsschema.ProcessorLimits{Limit: 50000, Weight: 1024}

	mockCtrl.EXPECT().State().Return(vm.StateRunning)
	mockCtrl.EXPECT().UpdateCPU(gomock.Any(), wantLimits).Return(nil)

	if _, err := svc.updateInternal(context.Background(), &task.UpdateTaskRequest{
		ID:        "pod-1",
		Resources: any,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestUpdate_CPUGroupAnnotation verifies that the CPUGroupID annotation is
// pulled out of the request and forwarded to vmController.UpdateCPUGroup.
// LinuxResources alone does not carry this value — it lives in annotations.
func TestUpdate_CPUGroupAnnotation(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.podControllers["pod-1"] = nil

	// Empty LinuxResources so we exercise the annotation branch alone.
	any, err := typeurl.MarshalAnyToProto(&specs.LinuxResources{})
	if err != nil {
		t.Fatalf("marshal resources: %v", err)
	}

	mockCtrl.EXPECT().State().Return(vm.StateRunning)
	mockCtrl.EXPECT().UpdateCPUGroup(gomock.Any(), "cpu-group-42").Return(nil)

	if _, err := svc.updateInternal(context.Background(), &task.UpdateTaskRequest{
		ID:          "pod-1",
		Resources:   any,
		Annotations: map[string]string{annotations.CPUGroupID: "cpu-group-42"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestUpdate_PolicyFragmentFailure verifies that a controller-side failure
// during policy-fragment update is wrapped and returned with the pod ID for
// diagnostics.
func TestUpdate_PolicyFragmentFailure(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.podControllers["pod-1"] = nil

	any, err := typeurl.MarshalAnyToProto(&ctrdtaskapi.PolicyFragment{Fragment: "test-fragment"})
	if err != nil {
		t.Fatalf("marshal fragment: %v", err)
	}

	mockCtrl.EXPECT().State().Return(vm.StateRunning)
	mockCtrl.EXPECT().UpdatePolicyFragment(gomock.Any(), gomock.Any()).Return(errVMUpdatePolicy)

	_, gotErr := svc.updateInternal(context.Background(), &task.UpdateTaskRequest{
		ID:        "pod-1",
		Resources: any,
	})
	if gotErr == nil {
		t.Fatal("expected error from UpdatePolicyFragment, got nil")
	}
	if !errors.Is(gotErr, errVMUpdatePolicy) {
		t.Errorf("expected error to wrap errVMUpdatePolicy, got %v", gotErr)
	}
}

// TestUpdate_MemoryFailure verifies that memory-update failures are wrapped.
func TestUpdate_MemoryFailure(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.podControllers["pod-1"] = nil

	limit := int64(1024 * 1024 * 1024)
	any, err := typeurl.MarshalAnyToProto(&specs.LinuxResources{
		Memory: &specs.LinuxMemory{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("marshal resources: %v", err)
	}

	mockCtrl.EXPECT().State().Return(vm.StateRunning)
	mockCtrl.EXPECT().UpdateMemory(gomock.Any(), gomock.Any()).Return(errVMUpdateMemory)

	_, gotErr := svc.updateInternal(context.Background(), &task.UpdateTaskRequest{
		ID:        "pod-1",
		Resources: any,
	})
	if gotErr == nil {
		t.Fatal("expected error from UpdateMemory, got nil")
	}
	if !errors.Is(gotErr, errVMUpdateMemory) {
		t.Errorf("expected error to wrap errVMUpdateMemory, got %v", gotErr)
	}
}

// TestUpdate_CPUFailure verifies that CPU-update failures are wrapped.
func TestUpdate_CPUFailure(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.podControllers["pod-1"] = nil

	quota := int64(10000)
	any, err := typeurl.MarshalAnyToProto(&specs.LinuxResources{
		CPU: &specs.LinuxCPU{Quota: &quota},
	})
	if err != nil {
		t.Fatalf("marshal resources: %v", err)
	}

	mockCtrl.EXPECT().State().Return(vm.StateRunning)
	mockCtrl.EXPECT().UpdateCPU(gomock.Any(), gomock.Any()).Return(errVMUpdateCPU)

	_, gotErr := svc.updateInternal(context.Background(), &task.UpdateTaskRequest{
		ID:        "pod-1",
		Resources: any,
	})
	if gotErr == nil {
		t.Fatal("expected error from UpdateCPU, got nil")
	}
	if !errors.Is(gotErr, errVMUpdateCPU) {
		t.Errorf("expected error to wrap errVMUpdateCPU, got %v", gotErr)
	}
}

// TestUpdate_CPUGroupFailure verifies that CPU-group-update failures are
// wrapped.
func TestUpdate_CPUGroupFailure(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.podControllers["pod-1"] = nil

	any, err := typeurl.MarshalAnyToProto(&specs.LinuxResources{})
	if err != nil {
		t.Fatalf("marshal resources: %v", err)
	}

	mockCtrl.EXPECT().State().Return(vm.StateRunning)
	mockCtrl.EXPECT().UpdateCPUGroup(gomock.Any(), "cpu-group-42").Return(errVMUpdateCPUGroup)

	_, gotErr := svc.updateInternal(context.Background(), &task.UpdateTaskRequest{
		ID:          "pod-1",
		Resources:   any,
		Annotations: map[string]string{annotations.CPUGroupID: "cpu-group-42"},
	})
	if gotErr == nil {
		t.Fatal("expected error from UpdateCPUGroup, got nil")
	}
	if !errors.Is(gotErr, errVMUpdateCPUGroup) {
		t.Errorf("expected error to wrap errVMUpdateCPUGroup, got %v", gotErr)
	}
}

// TestUpdate_UnsupportedResource verifies that an unsupported resource type
// is rejected with an InvalidArgument-wrapped error rather than panicking
// in the type switch.
func TestUpdate_UnsupportedResource(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.podControllers["pod-1"] = nil

	// Use ShutdownRequest as an arbitrary marshallable type the service
	// does not know how to dispatch.
	any, err := typeurl.MarshalAnyToProto(&task.ShutdownRequest{ID: "ignored"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	mockCtrl.EXPECT().State().Return(vm.StateRunning)

	_, gotErr := svc.updateInternal(context.Background(), &task.UpdateTaskRequest{
		ID:        "pod-1",
		Resources: any,
	})
	if gotErr == nil {
		t.Fatal("expected error for unsupported resource, got nil")
	}
	if !errors.Is(gotErr, errdefs.ErrInvalidArgument) {
		t.Errorf("expected error to wrap ErrInvalidArgument, got %v", gotErr)
	}
	if !strings.Contains(gotErr.Error(), "unsupported resource type") {
		t.Errorf("expected error to contain %q, got %q", "unsupported resource type", gotErr.Error())
	}
}

// ─── enrichNotFoundError tests ────────────────────────────────────────────

// TestEnrichNotFoundError_PassesThroughNonNotFound verifies that errors that
// are not in any of the recognized "not-found" categories pass through
// unwrapped; otherwise every guest-side error would be misclassified as
// missing.
func TestEnrichNotFoundError_PassesThroughNonNotFound(t *testing.T) {
	t.Parallel()
	in := errors.New("some unrelated error")
	out := enrichNotFoundError(in)
	if !errors.Is(out, in) {
		t.Errorf("expected pass-through, got %v", out)
	}
}

// TestEnrichNotFoundError_WrapsErrdefsNotFound verifies that an error already
// tagged with errdefs.ErrNotFound is returned with the same sentinel still
// reachable via errors.Is.
func TestEnrichNotFoundError_WrapsErrdefsNotFound(t *testing.T) {
	t.Parallel()
	in := errdefs.ErrNotFound
	out := enrichNotFoundError(in)
	if !errors.Is(out, errdefs.ErrNotFound) {
		t.Errorf("expected output to wrap ErrNotFound, got %v", out)
	}
}

// ─── Container delegation success paths ───────────────────────────────────

// swapGetContainerController replaces the package-level getContainerController
// seam with one that always yields mc, restoring the original when the test
// ends.
//
// Tests that use this helper MUST NOT call t.Parallel(): getContainerController
// is a package-level variable, so overriding it concurrently would race the
// real lookups exercised by the parallel guard tests in this file. Go's testing
// package never runs serial tests alongside the bodies of parallel ones, so a
// serial test is safe to swap the global and restore it via t.Cleanup before
// the parallel phase resumes.
func swapGetContainerController(t *testing.T, mc containerController) {
	t.Helper()
	orig := getContainerController
	t.Cleanup(func() { getContainerController = orig })
	getContainerController = func(*Service, string) (containerController, error) {
		return mc, nil
	}
}

// TestPids_Success verifies that pidsInternal forwards to the container
// controller and returns the processes it reports verbatim.
func TestPids_Success(t *testing.T) {
	svc, mockVM := newTestService(t)
	mockVM.EXPECT().State().Return(vm.StateRunning)

	mockCtr := mocks.NewMockcontainerController(gomock.NewController(t))
	swapGetContainerController(t, mockCtr)

	want := []*containerdtypes.ProcessInfo{{Pid: 42}, {Pid: 99}}
	mockCtr.EXPECT().Pids(gomock.Any()).Return(want, nil)

	resp, err := svc.pidsInternal(context.Background(), &task.PidsRequest{ID: "ctr-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Processes) != len(want) {
		t.Fatalf("Processes len = %d, want %d", len(resp.Processes), len(want))
	}
	if resp.Processes[0].Pid != 42 || resp.Processes[1].Pid != 99 {
		t.Errorf("Processes pids = [%d %d], want [42 99]", resp.Processes[0].Pid, resp.Processes[1].Pid)
	}
}

// TestStats_Success verifies the container-level stats path: statsInternal
// fetches the container's stats and marshals them into the response. The
// container is not registered as a pod, so no VM stats are attached.
func TestStats_Success(t *testing.T) {
	svc, mockVM := newTestService(t)
	mockVM.EXPECT().State().Return(vm.StateRunning)

	mockCtr := mocks.NewMockcontainerController(gomock.NewController(t))
	swapGetContainerController(t, mockCtr)

	mockCtr.EXPECT().Stats(gomock.Any()).Return(&stats.Statistics{}, nil)

	resp, err := svc.statsInternal(context.Background(), &task.StatsRequest{ID: "ctr-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.Stats == nil {
		t.Fatal("expected non-nil stats in response")
	}
}

// TestDeleteProcess_Success verifies the exec-deletion path: deleteInternal
// forwards the exec ID to the container controller and maps the returned
// process status onto the delete response. The forwarded ExecID is asserted so
// a regression that drops or swaps it is caught.
func TestDeleteProcess_Success(t *testing.T) {
	svc, _ := newTestService(t)

	mockCtr := mocks.NewMockcontainerController(gomock.NewController(t))
	swapGetContainerController(t, mockCtr)

	status := &task.StateResponse{Pid: 7, ExitStatus: 137}
	mockCtr.EXPECT().DeleteProcess(gomock.Any(), "exec-1").Return(status, nil)

	resp, err := svc.deleteInternal(context.Background(), &task.DeleteRequest{ID: "ctr-1", ExecID: "exec-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Pid != 7 || resp.ExitStatus != 137 {
		t.Errorf("resp = {Pid:%d ExitStatus:%d}, want {Pid:7 ExitStatus:137}", resp.Pid, resp.ExitStatus)
	}
}

// TestKill_Success verifies the single-container kill path: killInternal
// forwards the exec ID, signal, and all=false to the container controller.
func TestKill_Success(t *testing.T) {
	svc, _ := newTestService(t)

	mockCtr := mocks.NewMockcontainerController(gomock.NewController(t))
	swapGetContainerController(t, mockCtr)

	mockCtr.EXPECT().KillProcess(gomock.Any(), "exec-1", uint32(9), false).Return(nil)

	if _, err := svc.killInternal(context.Background(), &task.KillRequest{
		ID:     "ctr-1",
		ExecID: "exec-1",
		Signal: 9,
		All:    false,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestUpdateContainer_Success verifies the container-level update path:
// updateInternal unmarshals the resources and forwards them to the container
// controller's Update. The container is not a pod, so the VM-update branch is
// not taken. The forwarded resources are asserted to catch a regression that
// passes the wrong payload.
func TestUpdateContainer_Success(t *testing.T) {
	svc, mockVM := newTestService(t)
	mockVM.EXPECT().State().Return(vm.StateRunning)

	mockCtr := mocks.NewMockcontainerController(gomock.NewController(t))
	swapGetContainerController(t, mockCtr)

	shares := uint64(512)
	any, err := typeurl.MarshalAnyToProto(&specs.LinuxResources{
		CPU: &specs.LinuxCPU{Shares: &shares},
	})
	if err != nil {
		t.Fatalf("marshal resources: %v", err)
	}

	mockCtr.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, resources interface{}) error {
			lr, ok := resources.(*specs.LinuxResources)
			if !ok {
				t.Fatalf("Update resources type = %T, want *specs.LinuxResources", resources)
			}
			if lr.CPU == nil || lr.CPU.Shares == nil || *lr.CPU.Shares != 512 {
				t.Errorf("Update resources CPU.Shares = %v, want 512", lr.CPU)
			}
			return nil
		})

	if _, err := svc.updateInternal(context.Background(), &task.UpdateTaskRequest{
		ID:        "ctr-1",
		Resources: any,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestStartContainer_Success verifies the init-process start path:
// startInternal starts the container (ExecID empty) and returns the pid the
// container controller reports.
func TestStartContainer_Success(t *testing.T) {
	svc, mockVM := newTestService(t)
	mockVM.EXPECT().State().Return(vm.StateRunning)

	mockCtr := mocks.NewMockcontainerController(gomock.NewController(t))
	swapGetContainerController(t, mockCtr)

	mockCtr.EXPECT().Start(gomock.Any(), gomock.Any()).Return(uint32(1234), nil)

	resp, err := svc.startInternal(context.Background(), &task.StartRequest{ID: "ctr-1", ExecID: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Pid != 1234 {
		t.Errorf("resp.Pid = %d, want 1234", resp.Pid)
	}
}
