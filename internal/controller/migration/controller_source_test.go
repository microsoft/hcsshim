//go:build windows && lcow

package migration

import (
	"errors"
	"testing"

	"github.com/containerd/errdefs"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/Microsoft/hcsshim/internal/builder/vm/lcow"
	"github.com/Microsoft/hcsshim/internal/controller/migration/mocks"
	save "github.com/Microsoft/hcsshim/internal/controller/migration/save"
	"github.com/Microsoft/hcsshim/internal/controller/pod"
	vmpkg "github.com/Microsoft/hcsshim/internal/controller/vm"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// migrationEnabledOptions returns sandbox options that permit live migration.
func migrationEnabledOptions() *lcow.SandboxOptions {
	return &lcow.SandboxOptions{LiveMigrationSupportEnabled: true}
}

// sourceOptions returns a valid PrepareSourceOptions bound to the given VM mock.
func sourceOptions(vm vmController) *PrepareSourceOptions {
	return &PrepareSourceOptions{
		InitOptions: InitOptions{
			SessionID:      "sess-1",
			Origin:         hcsschema.MigrationOriginSource,
			VMController:   vm,
			PodControllers: map[string]*pod.Controller{},
		},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// PrepareSource
// ─────────────────────────────────────────────────────────────────────────────

// TestPrepareSource_RejectsInvalidArgs verifies a half-specified request is
// refused before any source-side state is touched.
func TestPrepareSource_RejectsInvalidArgs(t *testing.T) {
	cases := map[string]*PrepareSourceOptions{
		"NilOptions":        nil,
		"EmptySessionID":    {InitOptions: InitOptions{VMController: &mocks.MockvmController{}, PodControllers: map[string]*pod.Controller{}}},
		"NilVMController":   {InitOptions: InitOptions{SessionID: "sess-1", PodControllers: map[string]*pod.Controller{}}},
		"NilPodControllers": {InitOptions: InitOptions{SessionID: "sess-1", VMController: &mocks.MockvmController{}}},
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			c := New()

			err := c.PrepareSource(t.Context(), opts)
			if !errors.Is(err, errdefs.ErrInvalidArgument) {
				t.Fatalf("expected ErrInvalidArgument, got %v", err)
			}
			if c.state != StateIdle {
				t.Errorf("expected state Idle after rejected args, got %s", c.state)
			}
		})
	}
}

// TestPrepareSource_RejectsWrongState verifies a session can only be armed from
// idle; a controller busy with another session is rejected.
func TestPrepareSource_RejectsWrongState(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New()
	c.state = StateSourceExported
	c.sessionID = "other"

	err := c.PrepareSource(t.Context(), sourceOptions(mocks.NewMockvmController(ctrl)))
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition, got %v", err)
	}
}

// TestPrepareSource_IdempotentSameSession verifies re-arming the same session is
// a no-op that does not touch the VM.
func TestPrepareSource_IdempotentSameSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New()
	c.state = StateSourcePrepared
	c.sessionID = "sess-1"

	// No VM calls are expected on a duplicate arm.
	if err := c.PrepareSource(t.Context(), sourceOptions(mocks.NewMockvmController(ctrl))); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.state != StateSourcePrepared {
		t.Errorf("expected state SourcePrepared, got %s", c.state)
	}
}

// TestPrepareSource_RejectsMigrationDisabled verifies a sandbox that was not
// created with live migration enabled cannot be armed.
func TestPrepareSource_RejectsMigrationDisabled(t *testing.T) {
	cases := map[string]*lcow.SandboxOptions{
		"NilOptions":      nil,
		"FeatureDisabled": {LiveMigrationSupportEnabled: false},
	}
	for name, sandboxOpts := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			vm := mocks.NewMockvmController(ctrl)
			vm.EXPECT().State().Return(vmpkg.StateRunning)
			vm.EXPECT().SandboxOptions().Return(sandboxOpts)

			c := New()
			err := c.PrepareSource(t.Context(), sourceOptions(vm))
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Fatalf("expected ErrFailedPrecondition, got %v", err)
			}
			if c.state != StateIdle {
				t.Errorf("expected state Idle, got %s", c.state)
			}
		})
	}
}

// TestPrepareSource_RejectsWrongVMState verifies the source cannot be armed
// unless the VM is running.
func TestPrepareSource_RejectsWrongVMState(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)
	vm.EXPECT().State().Return(vmpkg.StateCreated).AnyTimes()

	c := New()
	if err := c.PrepareSource(t.Context(), sourceOptions(vm)); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition, got %v", err)
	}
	if c.state != StateIdle {
		t.Errorf("expected state Idle, got %s", c.state)
	}
}

// TestPrepareSource_InitializeError verifies a failure arming the VM leaves the
// controller idle so the session can be retried.
func TestPrepareSource_InitializeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)
	vm.EXPECT().State().Return(vmpkg.StateRunning)
	vm.EXPECT().SandboxOptions().Return(migrationEnabledOptions())
	vm.EXPECT().InitializeLiveMigrationOnSource(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	c := New()
	if err := c.PrepareSource(t.Context(), sourceOptions(vm)); err == nil {
		t.Fatal("expected error, got nil")
	}
	if c.state != StateIdle {
		t.Errorf("expected state Idle after failure, got %s", c.state)
	}
}

// TestPrepareSource_Success verifies a successful arm records the session, stamps
// the origin onto the (defaulted) migration options, and advances the state.
func TestPrepareSource_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)
	vm.EXPECT().State().Return(vmpkg.StateRunning)
	vm.EXPECT().SandboxOptions().Return(migrationEnabledOptions())
	vm.EXPECT().InitializeLiveMigrationOnSource(gomock.Any(), gomock.Any()).Return(nil)

	c := New()
	opts := sourceOptions(vm)
	if err := c.PrepareSource(t.Context(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.state != StateSourcePrepared {
		t.Errorf("expected state SourcePrepared, got %s", c.state)
	}
	if c.sessionID != "sess-1" || c.origin != hcsschema.MigrationOriginSource || c.vmController != vm {
		t.Errorf("session state not bound: %+v", c)
	}
	// MigrationOpts is defaulted when nil and stamped with the origin.
	if opts.MigrationOpts == nil || opts.MigrationOpts.Origin != hcsschema.MigrationOriginSource {
		t.Errorf("expected migration options stamped with source origin, got %+v", opts.MigrationOpts)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ExportState
// ─────────────────────────────────────────────────────────────────────────────

// TestExportState_RejectsSessionMismatch verifies a snapshot is only produced
// for the active session.
func TestExportState_RejectsSessionMismatch(t *testing.T) {
	c := New()
	c.state = StateSourcePrepared
	c.sessionID = "sess-1"

	env, err := c.ExportState(t.Context(), "other")
	if !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
	if env != nil {
		t.Errorf("expected nil envelope on failure, got %+v", env)
	}
}

// TestExportState_RejectsWrongState verifies a snapshot is only produced from a
// prepared (or already-exported) source.
func TestExportState_RejectsWrongState(t *testing.T) {
	c := New()
	c.sessionID = "sess-1"

	env, err := c.ExportState(t.Context(), "sess-1")
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition, got %v", err)
	}
	if env != nil {
		t.Errorf("expected nil envelope on failure, got %+v", env)
	}
}

// TestExportState_VMSaveError verifies a failure saving the VM aborts the export
// without advancing the state.
func TestExportState_VMSaveError(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)
	vm.EXPECT().Save(gomock.Any()).Return(nil, errors.New("boom"))

	c := New()
	c.state = StateSourcePrepared
	c.sessionID = "sess-1"
	c.vmController = vm
	c.podControllers = map[string]*pod.Controller{}

	if _, err := c.ExportState(t.Context(), "sess-1"); err == nil {
		t.Fatal("expected error, got nil")
	}
	if c.state != StateSourcePrepared {
		t.Errorf("expected state SourcePrepared after failure, got %s", c.state)
	}
}

// TestExportState_Success verifies the produced envelope is self-describing and
// carries the VM payload, and that the source advances to exported.
func TestExportState_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmAny := &anypb.Any{TypeUrl: "type.example/vm", Value: []byte("vm-state")}
	vm := mocks.NewMockvmController(ctrl)
	vm.EXPECT().Save(gomock.Any()).Return(vmAny, nil)

	c := New()
	c.state = StateSourcePrepared
	c.sessionID = "sess-1"
	c.vmController = vm
	c.podControllers = map[string]*pod.Controller{}

	env, err := c.ExportState(t.Context(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.GetTypeUrl() != save.TypeURL {
		t.Errorf("expected type URL %q, got %q", save.TypeURL, env.GetTypeUrl())
	}

	got := &save.Payload{}
	if err := proto.Unmarshal(env.GetValue(), got); err != nil {
		t.Fatalf("unmarshal saved payload: %v", err)
	}
	if got.GetSchemaVersion() != save.SchemaVersion {
		t.Errorf("expected schema version %d, got %d", save.SchemaVersion, got.GetSchemaVersion())
	}
	if got.GetVm().GetTypeUrl() != vmAny.TypeUrl || string(got.GetVm().GetValue()) != string(vmAny.Value) {
		t.Errorf("vm payload not preserved: %+v", got.GetVm())
	}
	if len(got.GetPods()) != 0 {
		t.Errorf("expected no pod payloads, got %d", len(got.GetPods()))
	}
	if c.state != StateSourceExported {
		t.Errorf("expected state SourceExported after export, got %s", c.state)
	}
}
