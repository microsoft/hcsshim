//go:build windows && lcow

package linuxcontainer

import (
	"context"
	"fmt"

	lcsave "github.com/Microsoft/hcsshim/internal/controller/linuxcontainer/save"
	"github.com/Microsoft/hcsshim/internal/controller/process"
	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"

	"github.com/Microsoft/go-winio/pkg/guid"
	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/api/runtime/task/v3"
	"github.com/containerd/errdefs"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Save serializes a running container's current state into a portable
// envelope that can be handed to a migration destination. It succeeds only
// when the container is running, the single stable state a live migration
// can be performed from. On success the source is frozen until it is resumed.
func (c *Controller) Save(ctx context.Context) (*anypb.Any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Only a running container is in a stable, migratable state.
	if c.state != StateRunning {
		return nil, fmt.Errorf("container %q in state %s; want %s", c.containerID, c.state, StateRunning)
	}

	// Capture the container's scalar bookkeeping into the snapshot.
	state := &lcsave.Payload{
		SchemaVersion:      lcsave.SchemaVersion,
		ContainerID:        c.containerID,
		GcsContainerID:     c.gcsContainerID,
		IoRetryTimeout:     durationpb.New(c.ioRetryTimeout),
		ScsiReservationIds: guidsToStrings(c.scsiResources),
	}

	// Record the rootfs layer reservations so the destination can re-create
	// the same read-only and scratch disks.
	if c.layers != nil {
		ls := &lcsave.Layers{
			LayersCombined: c.layers.layersCombined,
			RootfsPath:     c.layers.rootfsPath,
			Scratch: &lcsave.LayerReservation{
				ReservationID: c.layers.scratch.id.String(),
				GuestPath:     c.layers.scratch.guestPath,
			},
		}

		ls.RoLayers = make([]*lcsave.LayerReservation, 0, len(c.layers.roLayers))
		for _, r := range c.layers.roLayers {
			ls.RoLayers = append(ls.RoLayers, &lcsave.LayerReservation{
				ReservationID: r.id.String(),
				GuestPath:     r.guestPath,
			})
		}
		state.Layers = ls
	}

	// Live migration only supports a container whose sole process is the
	// init process; reject a missing init process or any additional execs.
	if _, ok := c.processes[""]; !ok || len(c.processes) > 1 {
		return nil, fmt.Errorf("container %q must have only the init process for live migration; has %d processes", c.containerID, len(c.processes))
	}

	// Snapshot the init process as an opaque payload the process controller
	// owns end to end.
	state.Processes = make(map[string]*anypb.Any, len(c.processes))
	for execID, p := range c.processes {
		ps, err := p.Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("save process %q/%q: %w", c.containerID, execID, err)
		}
		state.Processes[execID] = ps
	}

	// Marshal and wrap the snapshot in a self-describing envelope.
	payload, err := proto.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal container saved state for %q: %w", c.containerID, err)
	}

	// Freeze the source until the migration is resumed or its VM is torn down.
	c.state = StateSourceMigrating

	log.G(ctx).WithField(logfields.SourceContainerID, c.containerID).Debug("container controller saved state")

	return &anypb.Any{TypeUrl: lcsave.TypeURL, Value: payload}, nil
}

// guidsToStrings encodes reservation GUIDs for the wire payload.
func guidsToStrings(in []guid.GUID) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, g := range in {
		out[i] = g.String()
	}
	return out
}

// Import reconstructs a container from an envelope produced by
// [Controller.Save]. The returned container carries the saved state but is
// not yet bound to a running VM, guest, or device controllers, so
// operational calls are rejected until [Controller.Resume]. Child processes
// are imported too but must each be resumed individually by the caller.
func Import(ctx context.Context, env *anypb.Any) (*Controller, error) {
	// Reject an empty or mistyped envelope before touching its bytes.
	if env == nil {
		return nil, fmt.Errorf("container saved-state envelope is nil")
	}

	if env.GetTypeUrl() != lcsave.TypeURL {
		return nil, fmt.Errorf("unsupported container saved-state type %q", env.GetTypeUrl())
	}

	// Decode and reject any payload this build cannot interpret.
	state := &lcsave.Payload{}
	if err := proto.Unmarshal(env.GetValue(), state); err != nil {
		return nil, fmt.Errorf("unmarshal container saved state: %w", err)
	}

	if v := state.GetSchemaVersion(); v != lcsave.SchemaVersion {
		return nil, fmt.Errorf("unsupported container saved-state schema version %d (want %d)", v, lcsave.SchemaVersion)
	}

	// Rehydrate into the destination-migrating state: state is restored but no
	// live VM/guest/device interfaces are bound, so operational calls are
	// rejected until Resume.
	c := &Controller{
		containerID:    state.GetContainerID(),
		gcsContainerID: state.GetGcsContainerID(),
		state:          StateDestinationMigrating,
		ioRetryTimeout: state.GetIoRetryTimeout().AsDuration(),
		plan9Resources: []guid.GUID{},
		devices:        []guid.GUID{},
		processes:      make(map[string]*process.Controller),
		terminatedCh:   make(chan struct{}),
	}

	scsiIDs, err := stringsToGuids(state.GetScsiReservationIds())
	if err != nil {
		return nil, fmt.Errorf("decode scsi reservation ids: %w", err)
	}
	c.scsiResources = scsiIDs

	// Rebuild the rootfs layer reservations captured at save time.
	if l := state.GetLayers(); l != nil {
		ls := &scsiLayers{
			layersCombined: l.GetLayersCombined(),
			rootfsPath:     l.GetRootfsPath(),
		}

		if sc := l.GetScratch(); sc != nil {
			id, err := guid.FromString(sc.GetReservationID())
			if err != nil {
				return nil, fmt.Errorf("decode scratch reservation id: %w", err)
			}
			ls.scratch = scsiReservation{id: id, guestPath: sc.GetGuestPath()}
		}

		for _, ro := range l.GetRoLayers() {
			id, err := guid.FromString(ro.GetReservationID())
			if err != nil {
				return nil, fmt.Errorf("decode ro layer reservation id: %w", err)
			}
			ls.roLayers = append(ls.roLayers, scsiReservation{id: id, guestPath: ro.GetGuestPath()})
		}

		c.layers = ls
	}

	// Import each saved process into its own migrating controller.
	// The caller resumes them individually.
	for execID, ps := range state.GetProcesses() {
		p, err := process.Import(ctx, ps, c.containerID)
		if err != nil {
			return nil, fmt.Errorf("import process %q/%q: %w", c.containerID, execID, err)
		}

		c.processes[execID] = p
	}

	log.G(ctx).WithField(logfields.SourceContainerID, c.containerID).Debug("container controller imported state")

	return c, nil
}

// Resume brings a migrating container back to the running state. On the
// destination it binds the live VM, guest, and device controllers, reattaches
// the init process along with its IO, begins watching for the process to exit,
// and republishes a TaskCreate event so containerd treats the migrated task as
// running locally. On the source it lifts the freeze applied by Save on the
// container and its processes, since the live bindings and running processes
// are still intact.
func (c *Controller) Resume(
	ctx context.Context,
	vmID string,
	gcsPodID string,
	guestMgr guest,
	scsiCtrl scsiController,
	plan9Ctrl plan9Controller,
	vpci vPCIController,
	events chan interface{},
) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Source rollback: bindings and running processes are intact, so just lift
	// the freeze that Save applied — on the container and each of its processes.
	if c.state == StateSourceMigrating {
		// The live process and IO are intact, so no guest reattach is needed
		// (nil guest container and events).
		for execID, p := range c.processes {
			if err := p.Resume(ctx, c.container, nil); err != nil {
				return fmt.Errorf("resume process %q in container %q: %w", execID, c.containerID, err)
			}
		}

		c.state = StateRunning
		return nil
	}

	// Reopen the guest-side container that survived the move inside the UVM.
	gcsContainer, err := guestMgr.OpenContainer(ctx, c.gcsContainerID)
	if err != nil {
		return fmt.Errorf("open gcs container %q: %w", c.gcsContainerID, err)
	}

	initProc, ok := c.processes[""]
	if !ok {
		_ = gcsContainer.Close()
		return fmt.Errorf("init process missing in container %q", c.containerID)
	}

	// Reattach the init process to its live guest counterpart and rewire its
	// IO. events is nil here because the container, not the process, owns
	// publishing the init process's TaskExit, which handleInitProcessExit
	// (started below) does after teardown.
	if err := initProc.Resume(ctx, gcsContainer, nil); err != nil {
		_ = gcsContainer.Close()
		return fmt.Errorf("resume init process in container %q: %w", c.gcsContainerID, err)
	}

	c.vmID = vmID
	c.gcsPodID = gcsPodID
	c.guest = guestMgr
	c.scsi = scsiCtrl
	c.plan9 = plan9Ctrl
	c.vpci = vpci
	c.container = gcsContainer
	c.state = StateRunning

	// Watch the init process so its exit tears the container down and
	// publishes TaskExit, exactly as a freshly started container would.
	go c.handleInitProcessExit(ctx, initProc, events)

	// Announce the migrated task to containerd using the bundle/IO/pid that
	// Patch seeded from the destination's create request.
	if events != nil {
		status := initProc.Status(true)

		log.G(ctx).WithFields(logrus.Fields{
			logfields.ContainerID: c.containerID,
			"pid":                 status.Pid,
		}).Info("container.Resume: republishing TaskCreate")

		events <- &eventstypes.TaskCreate{
			ContainerID: c.containerID,
			Bundle:      status.Bundle,
			IO: &eventstypes.TaskIO{
				Stdin:    status.Stdin,
				Stdout:   status.Stdout,
				Stderr:   status.Stderr,
				Terminal: status.Terminal,
			},
			Pid: status.Pid,
		}
	}

	log.G(ctx).WithField(logfields.DestinationContainerID, c.containerID).Debug("container controller resumed state")
	return nil
}

// stringsToGuids decodes wire-format reservation GUIDs back into GUIDs.
func stringsToGuids(in []string) ([]guid.GUID, error) {
	if len(in) == 0 {
		return nil, nil
	}

	out := make([]guid.GUID, 0, len(in))
	for _, s := range in {
		g, err := guid.FromString(s)
		if err != nil {
			return nil, fmt.Errorf("parse guid %q: %w", s, err)
		}
		out = append(out, g)
	}

	return out, nil
}

// ContainerID returns the containerd-visible identifier for this container.
func (c *Controller) ContainerID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.containerID
}

// Patch updates an imported container to match the destination host's create
// request, readying it for [Controller.Resume]. It repoints every layer
// reservation at the destination's local VHD paths, reopens the init
// process's IO and bundle, and finally adopts the destination container ID.
// It is valid only while the container is migrating, and only for a container
// whose sole process is the init process.
func (c *Controller) Patch(ctx context.Context, scsiCtrl scsiController, request *task.CreateTaskRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateDestinationMigrating {
		return fmt.Errorf("container %s is in state %s; cannot patch: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	// Repoint every saved layer reservation at the destination host's VHDs
	// so future SCSI operations resolve to local disks.
	if c.layers != nil {
		if scsiCtrl == nil {
			return fmt.Errorf("scsi controller is required to patch container %q layers", c.containerID)
		}

		lcowLayers, err := layers.ParseLCOWLayers(request.Rootfs, nil)
		if err != nil {
			return fmt.Errorf("parse destination lcow layers: %w", err)
		}

		if got, want := len(lcowLayers.Layers), len(c.layers.roLayers); got != want {
			return fmt.Errorf("ro layer count mismatch: got %d, want %d", got, want)
		}

		for i, ro := range c.layers.roLayers {
			// Resolve to the canonical (volume-prefixed) path, exactly as
			// allocateLayers did at create time, so the SCSI controller keys
			// this disk the same way and dedupes later Reserve calls for it.
			hp, err := resolvePath(lcowLayers.Layers[i].VHDPath)
			if err != nil {
				return fmt.Errorf("resolve ro layer %d host path: %w", i, err)
			}

			// Update the disk path to local VHD on this destination host.
			if err := scsiCtrl.UpdateDiskHostPath(ctx, ro.id, hp); err != nil {
				return fmt.Errorf("patch ro layer %d: %w", i, err)
			}
		}

		scratchHP, err := resolvePath(lcowLayers.ScratchVHDPath)
		if err != nil {
			return fmt.Errorf("resolve scratch host path: %w", err)
		}

		// Update the disk path to local VHD on this destination host.
		if err := scsiCtrl.UpdateDiskHostPath(ctx, c.layers.scratch.id, scratchHP); err != nil {
			return fmt.Errorf("patch scratch layer: %w", err)
		}
	}

	// Re-establish IO for the init process from the destination's request.
	// Live migration carries only the init process: execs are rejected at the
	// source, and the destination's CreateTaskRequest describes the init
	// process alone, so it is the only process we can patch here.
	initProc, ok := c.processes[""]
	if !ok {
		return fmt.Errorf("init process missing in container %q", c.containerID)
	}

	if len(c.processes) > 1 {
		return fmt.Errorf("container %q has %d processes; live migration only supports the init process", c.containerID, len(c.processes))
	}

	if err := initProc.Patch(ctx, request.ID, &process.CreateOptions{
		Bundle:   request.Bundle,
		Terminal: request.Terminal,
		Stdin:    request.Stdin,
		Stdout:   request.Stdout,
		Stderr:   request.Stderr,
	}); err != nil {
		return fmt.Errorf("patch init process in container %q: %w", c.containerID, err)
	}

	log.G(ctx).WithFields(logrus.Fields{
		logfields.SourceContainerID:      c.containerID,
		logfields.DestinationContainerID: request.ID,
	}).Debug("patched container resource paths")

	// Adopt the destination's container ID last so a partial failure above
	// leaves the controller addressable for a retry.
	c.containerID = request.ID

	return nil
}

// AbortMigrated discards an imported-but-never-resumed container: it drains
// each imported process, marks the container stopped, and publishes a
// synthetic TaskExit so containerd will accept a Delete. It is a no-op once
// the container has left the migrating state.
func (c *Controller) AbortMigrated(ctx context.Context, events chan interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateDestinationMigrating {
		return
	}

	log.G(ctx).WithField(logfields.DestinationContainerID, c.containerID).Debug("aborting migrated container")

	// Tear down each imported process and the container handle before
	// reporting the container as exited.
	for _, proc := range c.processes {
		proc.AbortMigrated(ctx)
	}

	c.state = StateStopped
	c.closeContainer()

	// Emit a synthetic exit for the init process so containerd unblocks Delete.
	initProc := c.processes[""]
	if events == nil || initProc == nil {
		return
	}

	status := initProc.Status(true)
	events <- &eventstypes.TaskExit{
		ContainerID: c.containerID,
		ID:          status.ExecID,
		Pid:         status.Pid,
		ExitStatus:  status.ExitStatus,
		ExitedAt:    status.ExitedAt,
	}
}
