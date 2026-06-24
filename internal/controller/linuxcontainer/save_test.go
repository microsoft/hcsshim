//go:build windows && lcow

package linuxcontainer

import (
	"errors"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/controller/linuxcontainer/mocks"
	lcsave "github.com/Microsoft/hcsshim/internal/controller/linuxcontainer/save"
	"github.com/Microsoft/hcsshim/internal/controller/process"
	procsave "github.com/Microsoft/hcsshim/internal/controller/process/save"

	"github.com/Microsoft/go-winio/pkg/guid"
	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/api/runtime/task/v3"
	containerdtypes "github.com/containerd/containerd/api/types"
	"github.com/containerd/errdefs"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
)

var errOpenContainer = errors.New("open container failed")

// buildProcessEnvelope wraps a minimal, valid process payload for the given
// exec ID so the container importer can reconstruct a migrating process.
func buildProcessEnvelope(t *testing.T, execID string) *anypb.Any {
	t.Helper()
	value, err := proto.Marshal(&procsave.Payload{
		SchemaVersion:  procsave.SchemaVersion,
		ExecID:         execID,
		IoRetryTimeout: durationpb.New(time.Second),
	})
	if err != nil {
		t.Fatalf("marshal process payload = %v", err)
	}
	return &anypb.Any{TypeUrl: procsave.TypeURL, Value: value}
}

// importedInitProcess returns an init process controller restored into the
// migrating state, ready to be patched, resumed, or aborted.
func importedInitProcess(t *testing.T) *process.Controller {
	t.Helper()
	p, err := process.Import(t.Context(), buildProcessEnvelope(t, ""), testContainerID)
	if err != nil {
		t.Fatalf("import init process = %v", err)
	}
	return p
}

// patchedInitProcess returns a migrating init process whose IO has been opened,
// mirroring an imported-and-patched-but-never-resumed process. Empty IO paths
// avoid real named-pipe connections.
func patchedInitProcess(t *testing.T) *process.Controller {
	t.Helper()
	p := importedInitProcess(t)
	if err := p.Patch(t.Context(), testContainerID, &process.CreateOptions{}); err != nil {
		t.Fatalf("patch init process = %v", err)
	}
	return p
}

// baseContainerPayload returns a fully valid container payload that individual
// tests mutate to exercise specific decode failures.
func baseContainerPayload(t *testing.T) *lcsave.Payload {
	t.Helper()
	scsiGUID, _ := guid.NewV4()
	scratchGUID, _ := guid.NewV4()
	roGUID, _ := guid.NewV4()
	return &lcsave.Payload{
		SchemaVersion:      lcsave.SchemaVersion,
		ContainerID:        "src-ctr",
		GcsContainerID:     "gcs-ctr",
		IoRetryTimeout:     durationpb.New(2 * time.Second),
		ScsiReservationIds: []string{scsiGUID.String()},
		Layers: &lcsave.Layers{
			LayersCombined: true,
			RootfsPath:     "/rootfs",
			Scratch:        &lcsave.LayerReservation{ReservationID: scratchGUID.String(), GuestPath: "/dev/scratch"},
			RoLayers:       []*lcsave.LayerReservation{{ReservationID: roGUID.String(), GuestPath: "/dev/ro0"}},
		},
		Processes: map[string]*anypb.Any{"": buildProcessEnvelope(t, "")},
	}
}

// containerEnvelope marshals a payload into the self-describing wire envelope.
func containerEnvelope(t *testing.T, p *lcsave.Payload) *anypb.Any {
	t.Helper()
	value, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("marshal container payload = %v", err)
	}
	return &anypb.Any{TypeUrl: lcsave.TypeURL, Value: value}
}

// --- Save ---

// TestSave_WrongState verifies that only a running container can be saved.
func TestSave_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateNotCreated, StateCreated, StateStopped, StateInvalid, StateDestinationMigrating, StateSourceMigrating}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			c, _, _, _, _ := newContainerTestController(t)
			c.state = state

			if _, err := c.Save(t.Context()); err == nil {
				t.Errorf("Save() = nil; want error for state %s", state)
			}
		})
	}
}

// TestSave_ProcessConstraints verifies that Save rejects a running container
// unless its sole process is the init process.
func TestSave_ProcessConstraints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		seedProc func(c *Controller)
	}{
		{name: "no init process", seedProc: func(c *Controller) {
			c.processes["exec-1"] = process.New(testContainerID, "exec-1", nil, 0)
		}},
		{name: "extra exec process", seedProc: func(c *Controller) {
			c.processes[""] = process.New(testContainerID, "", nil, 0)
			c.processes["exec-1"] = process.New(testContainerID, "exec-1", nil, 0)
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _, _, _, _ := newContainerTestController(t)
			c.state = StateRunning
			tc.seedProc(c)

			if _, err := c.Save(t.Context()); err == nil {
				t.Error("Save() = nil; want error for invalid process set")
			}
		})
	}
}

// TestSave_ProcessSaveFails verifies that a failure to save the init process
// (here, because it is not running) surfaces from Save.
func TestSave_ProcessSaveFails(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	c.state = StateRunning
	// process.New starts in StateNotCreated, so its Save fails.
	c.processes[""] = process.New(testContainerID, "", nil, 0)

	if _, err := c.Save(t.Context()); err == nil {
		t.Error("Save() = nil; want error when init process cannot be saved")
	}
}

// --- Import ---

// TestImport_InvalidEnvelope verifies that Import rejects malformed or
// incompatible envelopes.
func TestImport_InvalidEnvelope(t *testing.T) {
	t.Parallel()
	badVersion := containerEnvelope(t, &lcsave.Payload{SchemaVersion: lcsave.SchemaVersion + 1})

	tests := []struct {
		name string
		env  *anypb.Any
	}{
		{name: "nil envelope", env: nil},
		{name: "wrong type url", env: &anypb.Any{TypeUrl: "type.microsoft.com/other"}},
		{name: "undecodable value", env: &anypb.Any{TypeUrl: lcsave.TypeURL, Value: []byte{0x08, 0xff}}},
		{name: "schema version mismatch", env: badVersion},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Import(t.Context(), tc.env); err == nil {
				t.Error("Import() = nil; want error")
			}
		})
	}
}

// TestImport_InvalidGUIDs verifies that Import rejects any reservation GUID it
// cannot decode, whether in the scsi list, scratch, or a read-only layer.
func TestImport_InvalidGUIDs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(p *lcsave.Payload)
	}{
		{name: "bad scsi id", mutate: func(p *lcsave.Payload) { p.ScsiReservationIds = []string{"not-a-guid"} }},
		{name: "bad scratch id", mutate: func(p *lcsave.Payload) { p.Layers.Scratch.ReservationID = "not-a-guid" }},
		{name: "bad ro layer id", mutate: func(p *lcsave.Payload) { p.Layers.RoLayers[0].ReservationID = "not-a-guid" }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := baseContainerPayload(t)
			tc.mutate(p)

			if _, err := Import(t.Context(), containerEnvelope(t, p)); err == nil {
				t.Error("Import() = nil; want error for invalid GUID")
			}
		})
	}
}

// TestImport_ProcessImportFails verifies that a bad embedded process envelope
// fails the whole container import.
func TestImport_ProcessImportFails(t *testing.T) {
	t.Parallel()
	p := baseContainerPayload(t)
	p.Processes[""] = &anypb.Any{TypeUrl: "type.microsoft.com/other"}

	if _, err := Import(t.Context(), containerEnvelope(t, p)); err == nil {
		t.Error("Import() = nil; want error for unimportable process")
	}
}

// TestImport_Succeeds verifies that Import reconstructs a migrating container
// carrying every saved field, including layers and the init process.
func TestImport_Succeeds(t *testing.T) {
	t.Parallel()
	p := baseContainerPayload(t)

	c, err := Import(t.Context(), containerEnvelope(t, p))
	if err != nil {
		t.Fatalf("Import() = %v; want nil", err)
	}
	if c.state != StateDestinationMigrating {
		t.Errorf("state = %s; want StateDestinationMigrating", c.state)
	}
	if c.containerID != p.ContainerID {
		t.Errorf("containerID = %q; want %q", c.containerID, p.ContainerID)
	}
	if c.gcsContainerID != p.GcsContainerID {
		t.Errorf("gcsContainerID = %q; want %q", c.gcsContainerID, p.GcsContainerID)
	}
	if c.ioRetryTimeout != 2*time.Second {
		t.Errorf("ioRetryTimeout = %s; want 2s", c.ioRetryTimeout)
	}
	if len(c.scsiResources) != 1 || c.scsiResources[0].String() != p.ScsiReservationIds[0] {
		t.Errorf("scsiResources = %v; want %v", c.scsiResources, p.ScsiReservationIds)
	}
	if c.layers == nil {
		t.Fatal("layers must be restored")
	}
	if !c.layers.layersCombined || c.layers.rootfsPath != "/rootfs" {
		t.Errorf("layers = %+v; want combined with rootfs /rootfs", c.layers)
	}
	if c.layers.scratch.id.String() != p.Layers.Scratch.ReservationID || c.layers.scratch.guestPath != "/dev/scratch" {
		t.Errorf("scratch = %+v; want %s", c.layers.scratch, p.Layers.Scratch.ReservationID)
	}
	if len(c.layers.roLayers) != 1 || c.layers.roLayers[0].id.String() != p.Layers.RoLayers[0].ReservationID {
		t.Errorf("roLayers = %+v; want %s", c.layers.roLayers, p.Layers.RoLayers[0].ReservationID)
	}
	if _, ok := c.processes[""]; !ok {
		t.Error("init process must be imported")
	}
	if c.terminatedCh == nil {
		t.Error("terminatedCh must be non-nil after Import")
	}
}

// TestImport_NoLayers verifies that a payload without layers imports cleanly.
func TestImport_NoLayers(t *testing.T) {
	t.Parallel()
	p := baseContainerPayload(t)
	p.Layers = nil

	c, err := Import(t.Context(), containerEnvelope(t, p))
	if err != nil {
		t.Fatalf("Import() = %v; want nil", err)
	}
	if c.layers != nil {
		t.Errorf("layers = %+v; want nil", c.layers)
	}
}

// --- Patch ---

// TestPatch_WrongState verifies that Patch only operates on a destination-migrating container.
func TestPatch_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateNotCreated, StateCreated, StateRunning, StateStopped, StateInvalid, StateSourceMigrating}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			c, scsiCtrl, _, _, _ := newContainerTestController(t)
			c.state = state

			err := c.Patch(t.Context(), scsiCtrl, &task.CreateTaskRequest{ID: "dest"})
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Errorf("Patch() = %v; want ErrFailedPrecondition", err)
			}
		})
	}
}

// TestPatch_LayerErrors verifies the layer-repointing failure modes: a missing
// scsi controller, unparsable rootfs, and a read-only layer count mismatch.
func TestPatch_LayerErrors(t *testing.T) {
	roGUID, _ := guid.NewV4()
	scratchGUID, _ := guid.NewV4()

	tests := []struct {
		name     string
		scsiNil  bool
		layers   *scsiLayers
		rootfs   []*containerdtypes.Mount
		stubPath bool
	}{
		{
			name:    "nil scsi controller",
			scsiNil: true,
			layers:  &scsiLayers{scratch: scsiReservation{id: scratchGUID}},
		},
		{
			name:   "unparsable rootfs",
			layers: &scsiLayers{scratch: scsiReservation{id: scratchGUID}},
			rootfs: nil,
		},
		{
			name:     "ro layer count mismatch",
			layers:   &scsiLayers{roLayers: []scsiReservation{{id: roGUID}, {id: roGUID}}, scratch: scsiReservation{id: scratchGUID}},
			rootfs:   []*containerdtypes.Mount{{Type: "lcow-layer", Source: `C:\scratch`, Options: []string{`parentLayerPaths=["C:\\layers\\base"]`}}},
			stubPath: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.stubPath {
				stubResolvePath(t)
			}
			c, scsiCtrl, _, _, _ := newContainerTestController(t)
			c.state = StateDestinationMigrating
			c.layers = tc.layers

			var sc scsiController = scsiCtrl
			if tc.scsiNil {
				sc = nil
			}

			err := c.Patch(t.Context(), sc, &task.CreateTaskRequest{ID: "dest", Rootfs: tc.rootfs})
			if err == nil {
				t.Error("Patch() = nil; want error")
			}
		})
	}
}

// TestPatch_UpdateDiskHostPathFails verifies that a SCSI disk repoint failure
// surfaces from Patch, whether it occurs on a read-only or the scratch layer.
// Not parallel: stubs the package-level resolvePath.
func TestPatch_UpdateDiskHostPathFails(t *testing.T) {
	stubResolvePath(t)

	roGUID, _ := guid.NewV4()
	scratchGUID, _ := guid.NewV4()
	wantErr := errors.New("update disk host path failed")
	rootfs := []*containerdtypes.Mount{{Type: "lcow-layer", Source: `C:\scratch`, Options: []string{`parentLayerPaths=["C:\\layers\\base"]`}}}

	tests := []struct {
		name   string
		expect func(scsiCtrl *mocks.MockscsiController)
	}{
		{
			name: "ro layer fails",
			expect: func(scsiCtrl *mocks.MockscsiController) {
				scsiCtrl.EXPECT().UpdateDiskHostPath(gomock.Any(), roGUID, gomock.Any()).Return(wantErr)
			},
		},
		{
			name: "scratch layer fails",
			expect: func(scsiCtrl *mocks.MockscsiController) {
				scsiCtrl.EXPECT().UpdateDiskHostPath(gomock.Any(), roGUID, gomock.Any()).Return(nil)
				scsiCtrl.EXPECT().UpdateDiskHostPath(gomock.Any(), scratchGUID, gomock.Any()).Return(wantErr)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, scsiCtrl, _, _, _ := newContainerTestController(t)
			c.state = StateDestinationMigrating
			c.layers = &scsiLayers{
				roLayers: []scsiReservation{{id: roGUID}},
				scratch:  scsiReservation{id: scratchGUID},
			}
			tc.expect(scsiCtrl)

			err := c.Patch(t.Context(), scsiCtrl, &task.CreateTaskRequest{ID: "dest", Rootfs: rootfs})
			if !errors.Is(err, wantErr) {
				t.Errorf("Patch() = %v; want %v", err, wantErr)
			}
		})
	}
}

// TestPatch_ProcessConstraints verifies that, with no layers to repoint, Patch
// still requires exactly the init process.
func TestPatch_ProcessConstraints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		seedProc func(c *Controller)
	}{
		{name: "no init process", seedProc: func(*Controller) {}},
		{name: "extra exec process", seedProc: func(c *Controller) {
			c.processes[""] = importedInitProcess(t)
			c.processes["exec-1"] = process.New(testContainerID, "exec-1", nil, 0)
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, scsiCtrl, _, _, _ := newContainerTestController(t)
			c.state = StateDestinationMigrating
			tc.seedProc(c)

			err := c.Patch(t.Context(), scsiCtrl, &task.CreateTaskRequest{ID: "dest"})
			if err == nil {
				t.Error("Patch() = nil; want error for invalid process set")
			}
		})
	}
}

// TestPatch_Succeeds verifies that Patch repoints the layers, re-opens init IO,
// and adopts the destination container ID. Empty IO paths avoid real pipes.
// Not parallel: stubs the package-level resolvePath.
func TestPatch_Succeeds(t *testing.T) {
	stubResolvePath(t)
	c, scsiCtrl, _, _, _ := newContainerTestController(t)
	c.state = StateDestinationMigrating

	roGUID, _ := guid.NewV4()
	scratchGUID, _ := guid.NewV4()
	c.layers = &scsiLayers{
		roLayers: []scsiReservation{{id: roGUID}},
		scratch:  scsiReservation{id: scratchGUID},
	}
	c.processes[""] = importedInitProcess(t)

	scsiCtrl.EXPECT().UpdateDiskHostPath(gomock.Any(), roGUID, gomock.Any()).Return(nil)
	scsiCtrl.EXPECT().UpdateDiskHostPath(gomock.Any(), scratchGUID, gomock.Any()).Return(nil)

	const destID = "dest-ctr-9999"
	rootfs := []*containerdtypes.Mount{{Type: "lcow-layer", Source: `C:\scratch`, Options: []string{`parentLayerPaths=["C:\\layers\\base"]`}}}

	if err := c.Patch(t.Context(), scsiCtrl, &task.CreateTaskRequest{ID: destID, Bundle: "/bundle", Rootfs: rootfs}); err != nil {
		t.Fatalf("Patch() = %v; want nil", err)
	}
	if c.containerID != destID {
		t.Errorf("containerID = %q; want %q", c.containerID, destID)
	}
	if c.state != StateDestinationMigrating {
		t.Errorf("state = %s; want StateDestinationMigrating", c.state)
	}
}

// --- Resume ---

// TestResume_OpenContainerFails verifies that Resume surfaces a guest
// OpenContainer failure before touching the init process.
func TestResume_OpenContainerFails(t *testing.T) {
	t.Parallel()
	c, scsiCtrl, plan9Ctrl, vpciCtrl, guestCtrl := newContainerTestController(t)
	c.state = StateDestinationMigrating

	guestCtrl.EXPECT().
		OpenContainer(gomock.Any(), c.gcsContainerID).
		Return(nil, errOpenContainer)

	err := c.Resume(t.Context(), testVMID, testPodID, guestCtrl, scsiCtrl, plan9Ctrl, vpciCtrl, nil)
	if !errors.Is(err, errOpenContainer) {
		t.Errorf("Resume() = %v; want %v", err, errOpenContainer)
	}
}

// TestResume_SourceRollback verifies that resuming a source-migrating container
// lifts the freeze and returns it to running without touching the guest.
func TestResume_SourceRollback(t *testing.T) {
	t.Parallel()
	c, scsiCtrl, plan9Ctrl, vpciCtrl, guestCtrl := newContainerTestController(t)
	c.state = StateSourceMigrating

	// No guest call is expected: the source keeps its live bindings.
	if err := c.Resume(t.Context(), testVMID, testPodID, guestCtrl, scsiCtrl, plan9Ctrl, vpciCtrl, nil); err != nil {
		t.Fatalf("Resume() = %v; want nil", err)
	}
	if c.state != StateRunning {
		t.Errorf("state = %s; want StateRunning", c.state)
	}
}

// --- AbortMigrated ---

// TestAbortMigrated_NoOp verifies that AbortMigrated leaves a non-migrating
// container untouched and publishes nothing.
func TestAbortMigrated_NoOp(t *testing.T) {
	t.Parallel()
	otherStates := []State{StateNotCreated, StateCreated, StateRunning, StateStopped, StateInvalid, StateSourceMigrating}

	for _, state := range otherStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			c, _, _, _, _ := newContainerTestController(t)
			c.state = state

			events := make(chan interface{}, 1)
			c.AbortMigrated(t.Context(), events)

			if c.state != state {
				t.Errorf("state = %s; want unchanged %s", c.state, state)
			}
			select {
			case <-events:
				t.Error("AbortMigrated published an event for a non-migrating container")
			default:
			}
		})
	}
}

// TestAbortMigrated_Succeeds verifies that AbortMigrated drains the init
// process, marks the container stopped, closes waiters, and publishes a
// synthetic TaskExit when an events channel is supplied.
func TestAbortMigrated_Succeeds(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	c.state = StateDestinationMigrating
	c.processes[""] = patchedInitProcess(t)

	events := make(chan interface{}, 1)
	c.AbortMigrated(t.Context(), events)

	if c.state != StateStopped {
		t.Errorf("state = %s; want StateStopped", c.state)
	}
	select {
	case <-c.terminatedCh:
	default:
		t.Error("terminatedCh should be closed after AbortMigrated")
	}
	select {
	case ev := <-events:
		if _, ok := ev.(*eventstypes.TaskExit); !ok {
			t.Errorf("event = %T; want *eventstypes.TaskExit", ev)
		}
	default:
		t.Error("AbortMigrated should publish a TaskExit event")
	}
}

// TestAbortMigrated_NoEventChannel verifies that AbortMigrated still tears the
// container down when no events channel is provided.
func TestAbortMigrated_NoEventChannel(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	c.state = StateDestinationMigrating
	c.processes[""] = importedInitProcess(t)

	c.AbortMigrated(t.Context(), nil)

	if c.state != StateStopped {
		t.Errorf("state = %s; want StateStopped", c.state)
	}
}

// TestAbortMigrated_NoInitProcess verifies that AbortMigrated tears the
// container down but publishes nothing when there is no init process.
func TestAbortMigrated_NoInitProcess(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	c.state = StateDestinationMigrating

	events := make(chan interface{}, 1)
	c.AbortMigrated(t.Context(), events)

	if c.state != StateStopped {
		t.Errorf("state = %s; want StateStopped", c.state)
	}
	select {
	case <-events:
		t.Error("AbortMigrated should not publish an event without an init process")
	default:
	}
}

// --- ContainerID ---

// TestContainerID verifies that ContainerID returns the current identifier.
func TestContainerID(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	if got := c.ContainerID(); got != testContainerID {
		t.Errorf("ContainerID() = %q; want %q", got, testContainerID)
	}
}

// --- guidsToStrings / stringsToGuids ---

// TestGUIDRoundTrip verifies the GUID encode/decode helpers, including nil
// handling and an undecodable string.
func TestGUIDRoundTrip(t *testing.T) {
	t.Parallel()

	if got := guidsToStrings(nil); got != nil {
		t.Errorf("guidsToStrings(nil) = %v; want nil", got)
	}
	if got, err := stringsToGuids(nil); err != nil || got != nil {
		t.Errorf("stringsToGuids(nil) = (%v, %v); want (nil, nil)", got, err)
	}
	if _, err := stringsToGuids([]string{"not-a-guid"}); err == nil {
		t.Error("stringsToGuids(invalid) = nil; want error")
	}

	g1, _ := guid.NewV4()
	g2, _ := guid.NewV4()
	encoded := guidsToStrings([]guid.GUID{g1, g2})
	decoded, err := stringsToGuids(encoded)
	if err != nil {
		t.Fatalf("stringsToGuids() = %v; want nil", err)
	}
	if len(decoded) != 2 || decoded[0] != g1 || decoded[1] != g2 {
		t.Errorf("round trip = %v; want [%v %v]", decoded, g1, g2)
	}
}
