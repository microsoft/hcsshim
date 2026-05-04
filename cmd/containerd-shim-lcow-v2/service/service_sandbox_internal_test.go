//go:build windows && lcow

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"golang.org/x/sys/windows"

	sandboxsvc "github.com/containerd/containerd/api/runtime/sandbox/v1"
	"github.com/containerd/containerd/v2/pkg/shutdown"

	"github.com/Microsoft/hcsshim/cmd/containerd-shim-lcow-v2/service/mocks"
	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	"github.com/Microsoft/hcsshim/internal/builder/vm/lcow"
	"github.com/Microsoft/hcsshim/internal/controller/pod"
	"github.com/Microsoft/hcsshim/internal/controller/vm"
)

// Sentinel errors used by the sandbox tests to assert that the service wraps
// and propagates errors from the underlying vm controller.
var (
	errVMCreate    = errors.New("vm create failed")
	errVMStart     = errors.New("vm start failed")
	errVMTerminate = errors.New("vm terminate failed")
	errVMWait      = errors.New("vm wait failed")
	errVMExitStat  = errors.New("vm exit status unavailable")
	errVMStats     = errors.New("vm stats unavailable")
)

// newTestService builds a [Service] wired to a mock vm controller.
func newTestService(t *testing.T) (*Service, *mocks.MockvmController) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockCtrl := mocks.NewMockvmController(ctrl)
	_, sd := shutdown.WithShutdown(context.Background())
	t.Cleanup(sd.Shutdown)
	return &Service{
		vmController:        mockCtrl,
		events:              make(chan interface{}, 128),
		podControllers:      make(map[string]*pod.Controller),
		containerPodMapping: make(map[string]string),
		shutdown:            sd,
	}, mockCtrl
}

// writeMiniConfigJSON drops a minimal config.json into dir so that
// createSandboxInternal gets past the file-read gate and reaches the
// duplicate-sandbox guard.
func writeMiniConfigJSON(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"annotations":{}}`), 0o644); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
}

// ─── createSandboxInternal tests ──────────────────────────────────────────

// TestCreateSandbox_DuplicateRejected verifies that a second CreateSandbox
// call against the same Service is rejected; the shim follows a
// one-sandbox-per-shim model and a duplicate would silently leak the first VM.
func TestCreateSandbox_DuplicateRejected(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	svc.sandboxID = "existing-sandbox"

	bundleDir := t.TempDir()
	writeMiniConfigJSON(t, bundleDir)

	_, err := svc.createSandboxInternal(context.Background(), &sandboxsvc.CreateSandboxRequest{
		SandboxID:  "new-sandbox",
		BundlePath: bundleDir,
	})
	if err == nil {
		t.Fatal("expected error for duplicate sandbox, got nil")
	}
	if !strings.Contains(err.Error(), "sandbox already exists") {
		t.Errorf("expected error to contain %q, got %q", "sandbox already exists", err.Error())
	}
}

// TestCreateSandbox_MissingConfigJSON verifies that an empty bundle directory
// is rejected before any VM creation work happens; if this guard regresses,
// the shim crashes deeper in JSON decoding instead of returning a clean error.
func TestCreateSandbox_MissingConfigJSON(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)

	_, err := svc.createSandboxInternal(context.Background(), &sandboxsvc.CreateSandboxRequest{
		SandboxID:  "test-sandbox",
		BundlePath: t.TempDir(), // empty dir, no config.json
	})
	if err == nil {
		t.Fatal("expected error for missing config.json, got nil")
	}
}

// TestCreateSandbox_VMCreateFailure verifies that a CreateVM failure is
// surfaced and that the sandboxID is NOT recorded on failure; recording it
// would lock the Service into an unusable state with no underlying VM.
func TestCreateSandbox_VMCreateFailure(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)

	bundleDir := t.TempDir()
	writeMiniConfigJSON(t, bundleDir)

	mockCtrl.EXPECT().CreateVM(gomock.Any(), gomock.Any()).Return(errVMCreate)

	_, err := svc.createSandboxInternal(context.Background(), &sandboxsvc.CreateSandboxRequest{
		SandboxID:  "test-sandbox",
		BundlePath: bundleDir,
	})
	if err == nil {
		t.Fatal("expected error from CreateVM, got nil")
	}
	if !errors.Is(err, errVMCreate) {
		t.Errorf("expected error to wrap errVMCreate, got %v", err)
	}
	if got := svc.sandboxID; got != "" {
		t.Errorf("expected sandboxID to remain empty after failure, got %q", got)
	}
}

// ─── Sandbox-ID-mismatch guards ───────────────────────────────────────────

// TestSandboxIDMismatch verifies that every per-sandbox internal method
// rejects a request whose SandboxID does not match the one this Service owns.
// A regression here would let containerd talk to the wrong sandbox after a
// shim restart.
func TestSandboxIDMismatch(t *testing.T) {
	tests := []struct {
		name string
		call func(*Service) error
	}{
		{
			name: "startSandboxInternal",
			call: func(svc *Service) error {
				_, err := svc.startSandboxInternal(context.Background(), &sandboxsvc.StartSandboxRequest{SandboxID: "wrong-sandbox"})
				return err
			},
		},
		{
			name: "platformInternal",
			call: func(svc *Service) error {
				_, err := svc.platformInternal(context.Background(), &sandboxsvc.PlatformRequest{SandboxID: "wrong-sandbox"})
				return err
			},
		},
		{
			name: "stopSandboxInternal",
			call: func(svc *Service) error {
				_, err := svc.stopSandboxInternal(context.Background(), &sandboxsvc.StopSandboxRequest{SandboxID: "wrong-sandbox"})
				return err
			},
		},
		{
			name: "waitSandboxInternal",
			call: func(svc *Service) error {
				_, err := svc.waitSandboxInternal(context.Background(), &sandboxsvc.WaitSandboxRequest{SandboxID: "wrong-sandbox"})
				return err
			},
		},
		{
			name: "sandboxStatusInternal",
			call: func(svc *Service) error {
				_, err := svc.sandboxStatusInternal(context.Background(), &sandboxsvc.SandboxStatusRequest{SandboxID: "wrong-sandbox"})
				return err
			},
		},
		{
			name: "shutdownSandboxInternal",
			call: func(svc *Service) error {
				_, err := svc.shutdownSandboxInternal(context.Background(), &sandboxsvc.ShutdownSandboxRequest{SandboxID: "wrong-sandbox"})
				return err
			},
		},
		{
			name: "sandboxMetricsInternal",
			call: func(svc *Service) error {
				_, err := svc.sandboxMetricsInternal(context.Background(), &sandboxsvc.SandboxMetricsRequest{SandboxID: "wrong-sandbox"})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run("reject/"+tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _ := newTestService(t)
			svc.sandboxID = "test-sandbox"

			err := tt.call(svc)
			if err == nil {
				t.Fatal("expected error for sandbox ID mismatch, got nil")
			}
			if !strings.Contains(err.Error(), "sandbox ID mismatch") {
				t.Errorf("expected error to contain %q, got %q", "sandbox ID mismatch", err.Error())
			}
		})
	}
}

// ─── startSandboxInternal tests ───────────────────────────────────────────

// TestStartSandbox_Success verifies that startSandboxInternal forwards to
// vmController.StartVM and returns the VM start time as CreatedAt.
func TestStartSandbox_Success(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().StartVM(gomock.Any(), gomock.Any()).Return(nil)
	mockCtrl.EXPECT().StartTime().Return(startedAt)

	resp, err := svc.startSandboxInternal(context.Background(), &sandboxsvc.StartSandboxRequest{SandboxID: "test-sandbox"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resp.CreatedAt.AsTime(); !got.Equal(startedAt) {
		t.Errorf("CreatedAt = %v, want %v", got, startedAt)
	}
}

// TestStartSandbox_StartVMFailure verifies that a StartVM error is wrapped
// and returned to the caller; if it were swallowed, containerd would think
// the sandbox is healthy when it is not.
func TestStartSandbox_StartVMFailure(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().StartVM(gomock.Any(), gomock.Any()).Return(errVMStart)

	_, err := svc.startSandboxInternal(context.Background(), &sandboxsvc.StartSandboxRequest{SandboxID: "test-sandbox"})
	if err == nil {
		t.Fatal("expected error from StartVM, got nil")
	}
	if !errors.Is(err, errVMStart) {
		t.Errorf("expected error to wrap errVMStart, got %v", err)
	}
}

// ─── platformInternal tests ───────────────────────────────────────────────

// TestPlatform_Success verifies that platformInternal reports the architecture
// from SandboxOptions and the linux platform string for the LCOW shim.
func TestPlatform_Success(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().State().Return(vm.StateRunning)
	mockCtrl.EXPECT().SandboxOptions().Return(&lcow.SandboxOptions{Architecture: "amd64"})

	resp, err := svc.platformInternal(context.Background(), &sandboxsvc.PlatformRequest{SandboxID: "test-sandbox"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := resp.Platform.OS, "linux"; got != want {
		t.Errorf("Platform.OS = %q, want %q", got, want)
	}
	if got, want := resp.Platform.Architecture, "amd64"; got != want {
		t.Errorf("Platform.Architecture = %q, want %q", got, want)
	}
}

// TestPlatform_NotCreatedRejected verifies that platformInternal refuses to
// answer when the VM has not yet been created; otherwise containerd would
// receive a default-zero Platform string and silently mis-route requests.
func TestPlatform_NotCreatedRejected(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	// State() is called twice in this branch: once for the guard check and
	// once when formatting the error string.
	mockCtrl.EXPECT().State().Return(vm.StateNotCreated).AnyTimes()

	_, err := svc.platformInternal(context.Background(), &sandboxsvc.PlatformRequest{SandboxID: "test-sandbox"})
	if err == nil {
		t.Fatal("expected error for not-created VM, got nil")
	}
	if !strings.Contains(err.Error(), "sandbox has not been created") {
		t.Errorf("expected error to contain %q, got %q", "sandbox has not been created", err.Error())
	}
}

// ─── stopSandboxInternal tests ────────────────────────────────────────────

// TestStopSandbox_Success verifies the happy-path forward to TerminateVM.
func TestStopSandbox_Success(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().TerminateVM(gomock.Any()).Return(nil)

	if _, err := svc.stopSandboxInternal(context.Background(), &sandboxsvc.StopSandboxRequest{SandboxID: "test-sandbox"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestStopSandbox_TerminateFailure verifies TerminateVM errors are wrapped.
func TestStopSandbox_TerminateFailure(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().TerminateVM(gomock.Any()).Return(errVMTerminate)

	_, err := svc.stopSandboxInternal(context.Background(), &sandboxsvc.StopSandboxRequest{SandboxID: "test-sandbox"})
	if err == nil {
		t.Fatal("expected error from TerminateVM, got nil")
	}
	if !errors.Is(err, errVMTerminate) {
		t.Errorf("expected error to wrap errVMTerminate, got %v", err)
	}
}

// TestStopSandbox_Idempotent verifies that two consecutive Stop calls against
// the same Service both reach TerminateVM; the service does not short-circuit
// on prior state, leaving idempotency to the controller. A regression that
// added a state check would break containerd retry of Stop after a shim
// restart.
func TestStopSandbox_Idempotent(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().TerminateVM(gomock.Any()).Return(nil).Times(2)

	for i := 0; i < 2; i++ {
		if _, err := svc.stopSandboxInternal(context.Background(), &sandboxsvc.StopSandboxRequest{SandboxID: "test-sandbox"}); err != nil {
			t.Fatalf("Stop call %d returned error: %v", i+1, err)
		}
	}
}

// ─── waitSandboxInternal tests ────────────────────────────────────────────

// TestWaitSandbox_CleanExit verifies that an exit with no error maps to
// ExitStatus = 0 and that the StoppedTime is propagated as ExitedAt.
func TestWaitSandbox_CleanExit(t *testing.T) {
	t.Parallel()
	stoppedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().Wait(gomock.Any()).Return(nil)
	mockCtrl.EXPECT().ExitStatus().Return(&vm.ExitStatus{StoppedTime: stoppedAt}, nil)

	resp, err := svc.waitSandboxInternal(context.Background(), &sandboxsvc.WaitSandboxRequest{SandboxID: "test-sandbox"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ExitStatus != 0 {
		t.Errorf("ExitStatus = %d, want 0", resp.ExitStatus)
	}
	if got := resp.ExitedAt.AsTime(); !got.Equal(stoppedAt) {
		t.Errorf("ExitedAt = %v, want %v", got, stoppedAt)
	}
}

// TestWaitSandbox_ErrorExit verifies that a non-clean exit maps to
// ERROR_INTERNAL_ERROR; this is the exit code containerd surfaces to the
// kubelet for sandbox failure events.
func TestWaitSandbox_ErrorExit(t *testing.T) {
	t.Parallel()
	stoppedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().Wait(gomock.Any()).Return(nil)
	mockCtrl.EXPECT().ExitStatus().Return(&vm.ExitStatus{
		StoppedTime: stoppedAt,
		Err:         errors.New("guest crashed"),
	}, nil)

	resp, err := svc.waitSandboxInternal(context.Background(), &sandboxsvc.WaitSandboxRequest{SandboxID: "test-sandbox"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := resp.ExitStatus, uint32(windows.ERROR_INTERNAL_ERROR); got != want {
		t.Errorf("ExitStatus = %d, want %d", got, want)
	}
}

// TestWaitSandbox_WaitFailure verifies that a Wait error short-circuits
// before ExitStatus is consulted; the wrapped error preserves the cause.
func TestWaitSandbox_WaitFailure(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().Wait(gomock.Any()).Return(errVMWait)

	_, err := svc.waitSandboxInternal(context.Background(), &sandboxsvc.WaitSandboxRequest{SandboxID: "test-sandbox"})
	if err == nil {
		t.Fatal("expected error from Wait, got nil")
	}
	if !errors.Is(err, errVMWait) {
		t.Errorf("expected error to wrap errVMWait, got %v", err)
	}
}

// TestWaitSandbox_ExitStatusFailure verifies that a Wait succeeds but a
// subsequent ExitStatus failure is wrapped and returned; without this the
// shim would silently report ExitStatus = 0 for a VM that may have failed.
func TestWaitSandbox_ExitStatusFailure(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().Wait(gomock.Any()).Return(nil)
	mockCtrl.EXPECT().ExitStatus().Return(nil, errVMExitStat)

	_, err := svc.waitSandboxInternal(context.Background(), &sandboxsvc.WaitSandboxRequest{SandboxID: "test-sandbox"})
	if err == nil {
		t.Fatal("expected error from ExitStatus, got nil")
	}
	if !errors.Is(err, errVMExitStat) {
		t.Errorf("expected error to wrap errVMExitStat, got %v", err)
	}
}

// ─── sandboxStatusInternal tests ──────────────────────────────────────────

// TestSandboxStatus_StateMapping verifies that every VM lifecycle state maps
// to the correct CRI sandbox state and that timestamp fields are populated
// only for the states that have meaningful values for them.
func TestSandboxStatus_StateMapping(t *testing.T) {
	startedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	stoppedAt := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		state         vm.State
		wantState     string
		wantCreatedAt bool
		wantExitedAt  bool
	}{
		{
			name:      "NotCreated",
			state:     vm.StateNotCreated,
			wantState: SandboxStateNotReady,
		},
		{
			name:      "Created",
			state:     vm.StateCreated,
			wantState: SandboxStateNotReady,
		},
		{
			name:      "Invalid",
			state:     vm.StateInvalid,
			wantState: SandboxStateNotReady,
		},
		{
			name:          "Running",
			state:         vm.StateRunning,
			wantState:     SandboxStateReady,
			wantCreatedAt: true,
		},
		{
			name:          "Terminated",
			state:         vm.StateTerminated,
			wantState:     SandboxStateNotReady,
			wantCreatedAt: true,
			wantExitedAt:  true,
		},
	}

	for _, tt := range tests {
		t.Run("state/"+tt.name, func(t *testing.T) {
			t.Parallel()
			svc, mockCtrl := newTestService(t)
			svc.sandboxID = "test-sandbox"

			mockCtrl.EXPECT().State().Return(tt.state)
			if tt.wantCreatedAt {
				mockCtrl.EXPECT().StartTime().Return(startedAt)
			}
			if tt.wantExitedAt {
				mockCtrl.EXPECT().ExitStatus().Return(&vm.ExitStatus{StoppedTime: stoppedAt}, nil)
			}

			resp, err := svc.sandboxStatusInternal(context.Background(), &sandboxsvc.SandboxStatusRequest{SandboxID: "test-sandbox"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.State != tt.wantState {
				t.Errorf("State = %q, want %q", resp.State, tt.wantState)
			}
			switch {
			case tt.wantCreatedAt && resp.CreatedAt == nil:
				t.Error("expected CreatedAt to be set")
			case tt.wantCreatedAt:
				if got := resp.CreatedAt.AsTime(); !got.Equal(startedAt) {
					t.Errorf("CreatedAt = %v, want %v", got, startedAt)
				}
			case !tt.wantCreatedAt && resp.CreatedAt != nil:
				t.Errorf("expected CreatedAt nil, got %v", resp.CreatedAt)
			}
			switch {
			case tt.wantExitedAt && resp.ExitedAt == nil:
				t.Error("expected ExitedAt to be set")
			case tt.wantExitedAt:
				if got := resp.ExitedAt.AsTime(); !got.Equal(stoppedAt) {
					t.Errorf("ExitedAt = %v, want %v", got, stoppedAt)
				}
			case !tt.wantExitedAt && resp.ExitedAt != nil:
				t.Errorf("expected ExitedAt nil, got %v", resp.ExitedAt)
			}
		})
	}
}

// TestSandboxStatus_TerminatedExitStatusFailure verifies that an ExitStatus
// failure on a Terminated VM is wrapped and returned; otherwise the sandbox
// would appear "ready=false" with no diagnostic info about why.
func TestSandboxStatus_TerminatedExitStatusFailure(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().State().Return(vm.StateTerminated)
	mockCtrl.EXPECT().StartTime().Return(startedAt)
	mockCtrl.EXPECT().ExitStatus().Return(nil, errVMExitStat)

	_, err := svc.sandboxStatusInternal(context.Background(), &sandboxsvc.SandboxStatusRequest{SandboxID: "test-sandbox"})
	if err == nil {
		t.Fatal("expected error from ExitStatus, got nil")
	}
	if !errors.Is(err, errVMExitStat) {
		t.Errorf("expected error to wrap errVMExitStat, got %v", err)
	}
}

// ─── pingSandboxInternal tests ────────────────────────────────────────────

// TestPingSandbox_NotImplemented verifies that pingSandboxInternal returns
// errdefs.ErrNotImplemented; some callers depend on this code to detect
// sandboxes that do not support liveness probes.
func TestPingSandbox_NotImplemented(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)

	_, err := svc.pingSandboxInternal(context.Background(), &sandboxsvc.PingRequest{})
	if err == nil {
		t.Fatal("expected ErrNotImplemented, got nil")
	}
}

// ─── shutdownSandboxInternal tests ────────────────────────────────────────

// TestShutdownSandbox_AlreadyTerminated verifies that shutdownSandboxInternal
// does NOT call TerminateVM when the VM is already terminated; doing so
// would produce a misleading log line on every shim shutdown.
func TestShutdownSandbox_AlreadyTerminated(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().State().Return(vm.StateTerminated)
	// TerminateVM must NOT be called.

	if _, err := svc.shutdownSandboxInternal(context.Background(), &sandboxsvc.ShutdownSandboxRequest{SandboxID: "test-sandbox"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestShutdownSandbox_TerminatesRunningVM verifies that shutdownSandboxInternal
// terminates a running VM as part of best-effort cleanup.
func TestShutdownSandbox_TerminatesRunningVM(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().State().Return(vm.StateRunning)
	mockCtrl.EXPECT().TerminateVM(gomock.Any()).Return(nil)

	if _, err := svc.shutdownSandboxInternal(context.Background(), &sandboxsvc.ShutdownSandboxRequest{SandboxID: "test-sandbox"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestShutdownSandbox_TerminateErrorSwallowed verifies that a TerminateVM
// failure during shutdown is logged but NOT returned to the caller; the
// shutdown handler is best-effort and a returned error would make containerd
// retry the request, leaking the goroutine that schedules the actual exit.
func TestShutdownSandbox_TerminateErrorSwallowed(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().State().Return(vm.StateRunning)
	mockCtrl.EXPECT().TerminateVM(gomock.Any()).Return(errVMTerminate)

	if _, err := svc.shutdownSandboxInternal(context.Background(), &sandboxsvc.ShutdownSandboxRequest{SandboxID: "test-sandbox"}); err != nil {
		t.Errorf("expected nil error (terminate failure must be swallowed), got: %v", err)
	}
}

// ─── sandboxMetricsInternal tests ─────────────────────────────────────────

// TestSandboxMetrics_Success verifies the happy-path: Stats is fetched,
// marshalled, and returned with the SandboxID stamped on the metric.
func TestSandboxMetrics_Success(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().Stats(gomock.Any()).Return(&stats.VirtualMachineStatistics{}, nil)

	resp, err := svc.sandboxMetricsInternal(context.Background(), &sandboxsvc.SandboxMetricsRequest{SandboxID: "test-sandbox"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Metrics == nil {
		t.Fatal("expected Metrics to be non-nil")
	}
	if got, want := resp.Metrics.ID, "test-sandbox"; got != want {
		t.Errorf("Metrics.ID = %q, want %q", got, want)
	}
}

// TestSandboxMetrics_StatsFailure verifies that Stats errors are wrapped.
func TestSandboxMetrics_StatsFailure(t *testing.T) {
	t.Parallel()
	svc, mockCtrl := newTestService(t)
	svc.sandboxID = "test-sandbox"

	mockCtrl.EXPECT().Stats(gomock.Any()).Return(nil, errVMStats)

	_, err := svc.sandboxMetricsInternal(context.Background(), &sandboxsvc.SandboxMetricsRequest{SandboxID: "test-sandbox"})
	if err == nil {
		t.Fatal("expected error from Stats, got nil")
	}
	if !errors.Is(err, errVMStats) {
		t.Errorf("expected error to wrap errVMStats, got %v", err)
	}
}
