//go:build windows && lcow

package migration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/containerd/containerd/api/runtime/task/v3"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/Microsoft/hcsshim/internal/controller/migration/mocks"
	save "github.com/Microsoft/hcsshim/internal/controller/migration/save"
	"github.com/Microsoft/hcsshim/internal/controller/pod"
	"github.com/Microsoft/hcsshim/internal/controller/vm"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/oci"
	hcsannotations "github.com/Microsoft/hcsshim/pkg/annotations"
)

// validImportPayload returns a decodable, current-schema sandbox payload with a
// VM blob and no pods, so import never has to construct a real pod controller.
func validImportPayload() *save.Payload {
	return &save.Payload{
		SchemaVersion: save.SchemaVersion,
		Vm:            &anypb.Any{TypeUrl: "type.example/vm", Value: []byte("vm")},
	}
}

// importEnvelope marshals a payload and wraps it in an envelope with the
// well-known type URL, matching what the source's ExportState emits.
func importEnvelope(t *testing.T, p *save.Payload) *anypb.Any {
	t.Helper()
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &anypb.Any{TypeUrl: save.TypeURL, Value: b}
}

// importOptions returns a valid ImportStateOptions bound to the given VM mock
// and saved-state envelope.
func importOptions(vmc vmController, saved *anypb.Any) *ImportStateOptions {
	return &ImportStateOptions{
		InitOptions: InitOptions{
			SessionID:      "sess-1",
			Origin:         hcsschema.MigrationOriginDestination,
			VMController:   vmc,
			PodControllers: map[string]*pod.Controller{},
		},
		SandboxID:           "sandbox-1",
		SavedState:          saved,
		ContainerPodMapping: map[string]string{},
	}
}

// importVM returns a VM mock reporting the not-created state that ImportState
// requires before rehydrating a snapshot.
func importVM(ctrl *gomock.Controller) *mocks.MockvmController {
	vmc := mocks.NewMockvmController(ctrl)
	vmc.EXPECT().State().Return(vm.StateNotCreated).AnyTimes()
	return vmc
}

// patchSpec builds a spec carrying the source-container annotation plus any extras.
func patchSpec(sourceContainerID string, extra map[string]string) specs.Spec {
	ann := map[string]string{hcsannotations.LiveMigrationSourceContainerID: sourceContainerID}
	for k, v := range extra {
		ann[k] = v
	}
	return specs.Spec{Annotations: ann}
}

// ─────────────────────────────────────────────────────────────────────────────
// ImportState
// ─────────────────────────────────────────────────────────────────────────────

// TestImportState_RejectsInvalidArgs verifies a half-specified request is
// refused before the controller is mutated.
func TestImportState_RejectsInvalidArgs(t *testing.T) {
	valid := func() *ImportStateOptions {
		return importOptions(&mocks.MockvmController{}, importEnvelope(t, validImportPayload()))
	}
	cases := map[string]*ImportStateOptions{
		"NilOptions":             nil,
		"EmptySessionID":         func() *ImportStateOptions { o := valid(); o.SessionID = ""; return o }(),
		"NilVMController":        func() *ImportStateOptions { o := valid(); o.VMController = nil; return o }(),
		"EmptySandboxID":         func() *ImportStateOptions { o := valid(); o.SandboxID = ""; return o }(),
		"NilPodControllers":      func() *ImportStateOptions { o := valid(); o.PodControllers = nil; return o }(),
		"NilContainerPodMapping": func() *ImportStateOptions { o := valid(); o.ContainerPodMapping = nil; return o }(),
		"NilSavedState":          func() *ImportStateOptions { o := valid(); o.SavedState = nil; return o }(),
		"WrongTypeURL":           func() *ImportStateOptions { o := valid(); o.SavedState = &anypb.Any{TypeUrl: "type.bogus"}; return o }(),
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			c := New()

			err := c.ImportState(t.Context(), opts)
			if !errors.Is(err, errdefs.ErrInvalidArgument) {
				t.Fatalf("expected ErrInvalidArgument, got %v", err)
			}
			if c.state != StateIdle {
				t.Errorf("expected state Idle after rejected args, got %s", c.state)
			}
		})
	}
}

// TestImportState_RejectsUndecodableState verifies a payload this build cannot
// decode is rejected before any controller state is rehydrated.
func TestImportState_RejectsUndecodableState(t *testing.T) {
	cases := map[string]*anypb.Any{
		"CorruptPayload": {TypeUrl: save.TypeURL, Value: []byte{0x08}},
		"SchemaMismatch": importEnvelope(t, &save.Payload{SchemaVersion: save.SchemaVersion + 1}),
	}
	for name, saved := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			c := New()

			if err := c.ImportState(t.Context(), importOptions(importVM(ctrl), saved)); err == nil {
				t.Fatal("expected error, got nil")
			}
			if c.state != StateIdle {
				t.Errorf("expected state Idle, got %s", c.state)
			}
		})
	}
}

// TestImportState_IdempotentSameSession verifies a repeat import for the active
// session is a no-op that does not re-import the VM.
func TestImportState_IdempotentSameSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New()
	c.state = StateDestinationImported
	c.sessionID = "sess-1"

	if err := c.ImportState(t.Context(), importOptions(importVM(ctrl), importEnvelope(t, validImportPayload()))); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestImportState_RejectsConflictingState verifies a snapshot cannot be imported
// while the controller is busy with another (non-idle) session.
func TestImportState_RejectsConflictingState(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New()
	c.state = StateDestinationPrepared
	c.sessionID = "other"

	err := c.ImportState(t.Context(), importOptions(importVM(ctrl), importEnvelope(t, validImportPayload())))
	if !errors.Is(err, errdefs.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

// TestImportState_RejectsWrongVMState verifies a snapshot cannot be imported
// unless the VM controller has not yet been created.
func TestImportState_RejectsWrongVMState(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmc := mocks.NewMockvmController(ctrl)
	vmc.EXPECT().State().Return(vm.StateRunning).AnyTimes()

	c := New()
	err := c.ImportState(t.Context(), importOptions(vmc, importEnvelope(t, validImportPayload())))
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition, got %v", err)
	}
}

// TestImportState_VMImportError verifies a failure rehydrating the VM aborts the
// import without advancing the state.
func TestImportState_VMImportError(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmc := importVM(ctrl)
	vmc.EXPECT().Import(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	c := New()
	if err := c.ImportState(t.Context(), importOptions(vmc, importEnvelope(t, validImportPayload()))); err == nil {
		t.Fatal("expected error, got nil")
	}
	if c.state != StateIdle {
		t.Errorf("expected state Idle after failure, got %s", c.state)
	}
}

// TestImportState_Success verifies a valid snapshot with no pods rehydrates the
// VM, binds the session, and advances the state.
func TestImportState_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmc := importVM(ctrl)
	vmc.EXPECT().Import(gomock.Any(), gomock.Any()).Return(nil)

	c := New()
	opts := importOptions(vmc, importEnvelope(t, validImportPayload()))
	if err := c.ImportState(t.Context(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.state != StateDestinationImported {
		t.Errorf("expected state DestinationImported, got %s", c.state)
	}
	if c.sessionID != "sess-1" || c.sandboxID != "sandbox-1" || c.vmController != vmc {
		t.Errorf("session state not bound: %+v", c)
	}
	if c.pendingPatches == nil || len(c.pendingPatches) != 0 {
		t.Errorf("expected empty pending set, got %+v", c.pendingPatches)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// PatchResourcePaths
// ─────────────────────────────────────────────────────────────────────────────

// TestPatchResourcePaths_RejectsInvalidArgs verifies a malformed request or spec
// is refused before the controller is touched.
func TestPatchResourcePaths_RejectsInvalidArgs(t *testing.T) {
	cases := map[string]struct {
		request *task.CreateTaskRequest
		spec    specs.Spec
	}{
		"NilRequest":              {nil, patchSpec("ctr-1", nil)},
		"EmptyRequestID":          {&task.CreateTaskRequest{ID: ""}, patchSpec("ctr-1", nil)},
		"NilAnnotations":          {&task.CreateTaskRequest{ID: "dst-1"}, specs.Spec{}},
		"MissingSourceAnnotation": {&task.CreateTaskRequest{ID: "dst-1"}, specs.Spec{Annotations: map[string]string{}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := New()

			err := c.PatchResourcePaths(t.Context(), tc.request, tc.spec)
			if !errors.Is(err, errdefs.ErrInvalidArgument) {
				t.Fatalf("expected ErrInvalidArgument, got %v", err)
			}
		})
	}
}

// TestPatchResourcePaths_RejectsWrongState verifies patching is only valid once
// the destination has imported a snapshot.
func TestPatchResourcePaths_RejectsWrongState(t *testing.T) {
	c := New() // StateIdle

	err := c.PatchResourcePaths(t.Context(), &task.CreateTaskRequest{ID: "dst-1"}, patchSpec("ctr-1", nil))
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition, got %v", err)
	}
}

// TestPatchResourcePaths_RejectsAlreadyPatched verifies a container that is no
// longer pending cannot be patched again.
func TestPatchResourcePaths_RejectsAlreadyPatched(t *testing.T) {
	c := New()
	c.state = StateDestinationImported
	c.pendingPatches = map[string]struct{}{}
	c.containerPodMapping = map[string]string{}

	err := c.PatchResourcePaths(t.Context(), &task.CreateTaskRequest{ID: "dst-1"}, patchSpec("ctr-1", nil))
	if !errors.Is(err, errdefs.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

// TestPatchResourcePaths_RejectsUnknownContainer verifies a pending container
// missing from the index is reported as not found.
func TestPatchResourcePaths_RejectsUnknownContainer(t *testing.T) {
	c := New()
	c.state = StateDestinationImported
	c.pendingPatches = map[string]struct{}{"ctr-1": {}}
	c.containerPodMapping = map[string]string{}

	err := c.PatchResourcePaths(t.Context(), &task.CreateTaskRequest{ID: "dst-1"}, patchSpec("ctr-1", nil))
	if !errors.Is(err, errdefs.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestPatchResourcePaths_RejectsAnnotationMismatch verifies a container-type
// annotation that contradicts the structural sandbox detection is rejected.
func TestPatchResourcePaths_RejectsAnnotationMismatch(t *testing.T) {
	c := New()
	c.state = StateDestinationImported
	c.pendingPatches = map[string]struct{}{"ctr-1": {}}
	c.containerPodMapping = map[string]string{"ctr-1": "pod-1"} // ctr-1 != pod-1, so not a sandbox
	c.podControllers = map[string]*pod.Controller{}

	// Annotation claims sandbox, contradicting the structural detection.
	spec := patchSpec("ctr-1", map[string]string{
		hcsannotations.KubernetesContainerType: string(oci.KubernetesContainerTypeSandbox),
	})

	err := c.PatchResourcePaths(t.Context(), &task.CreateTaskRequest{ID: "dst-1"}, spec)
	if !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

// TestPatchResourcePaths_SCSIControllerError verifies a failure obtaining the
// SCSI controller surfaces before the pod is patched.
func TestPatchResourcePaths_SCSIControllerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmc := mocks.NewMockvmController(ctrl)
	vmc.EXPECT().SCSIController(gomock.Any()).Return(nil, errors.New("boom"))

	c := New()
	c.state = StateDestinationImported
	c.pendingPatches = map[string]struct{}{"ctr-1": {}}
	c.containerPodMapping = map[string]string{"ctr-1": "pod-1"}
	c.podControllers = map[string]*pod.Controller{}
	c.vmController = vmc

	if err := c.PatchResourcePaths(t.Context(), &task.CreateTaskRequest{ID: "dst-1"}, patchSpec("ctr-1", nil)); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// PrepareDestination
// ─────────────────────────────────────────────────────────────────────────────

// TestPrepareDestination_RejectsWrongState verifies the destination VM is only
// built from an imported controller.
func TestPrepareDestination_RejectsWrongState(t *testing.T) {
	c := New() // StateIdle
	c.sessionID = "sess-1"

	if err := c.PrepareDestination(t.Context(), "sess-1", nil); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition, got %v", err)
	}
}

// TestPrepareDestination_RejectsPendingPatches verifies the VM is not built while
// any imported container is still awaiting a patch.
func TestPrepareDestination_RejectsPendingPatches(t *testing.T) {
	c := New()
	c.state = StateDestinationImported
	c.sessionID = "sess-1"
	c.pendingPatches = map[string]struct{}{"ctr-1": {}, "ctr-2": {}}

	err := c.PrepareDestination(t.Context(), "sess-1", nil)
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition, got %v", err)
	}
	if !strings.Contains(err.Error(), "ctr-1") || !strings.Contains(err.Error(), "ctr-2") {
		t.Errorf("expected error to list pending containers, got: %v", err)
	}
}

// TestPrepareDestination_CreateVMError verifies a failure creating the VM aborts
// the preparation without advancing the state.
func TestPrepareDestination_CreateVMError(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmc := mocks.NewMockvmController(ctrl)
	vmc.EXPECT().CreateVM(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	c := New()
	c.state = StateDestinationImported
	c.sandboxID = "sandbox-1"
	c.sessionID = "sess-1"
	c.pendingPatches = map[string]struct{}{}
	c.vmController = vmc

	if err := c.PrepareDestination(t.Context(), "sess-1", nil); err == nil {
		t.Fatal("expected error, got nil")
	}
	if c.state != StateDestinationImported {
		t.Errorf("expected state DestinationImported after failure, got %s", c.state)
	}
}

// TestPrepareDestination_Success verifies a successful preparation stamps the
// origin onto the (defaulted) options, builds the VM, and advances the state.
func TestPrepareDestination_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	vmc := mocks.NewMockvmController(ctrl)

	var gotOpts *vm.CreateOptions
	vmc.EXPECT().CreateVM(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, o *vm.CreateOptions) error {
			gotOpts = o
			return nil
		})
	vmc.EXPECT().Patch(gomock.Any()).Return(nil)

	c := New()
	c.state = StateDestinationImported
	c.sandboxID = "sandbox-1"
	c.sessionID = "sess-1"
	c.origin = hcsschema.MigrationOriginDestination
	c.pendingPatches = map[string]struct{}{}
	c.vmController = vmc

	if err := c.PrepareDestination(t.Context(), "sess-1", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.state != StateDestinationPrepared {
		t.Errorf("expected state DestinationPrepared, got %s", c.state)
	}
	if gotOpts == nil || gotOpts.ID != "sandbox-1@vm" {
		t.Fatalf("unexpected create options: %+v", gotOpts)
	}
	// MigrationOptions is defaulted when nil and stamped with the origin.
	if gotOpts.MigrationOptions == nil || gotOpts.MigrationOptions.Origin != hcsschema.MigrationOriginDestination {
		t.Errorf("expected migration options stamped with destination origin, got %+v", gotOpts.MigrationOptions)
	}
}
