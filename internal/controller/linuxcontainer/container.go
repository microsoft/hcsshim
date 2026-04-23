//go:build windows && lcow

package linuxcontainer

import (
	"context"
	"fmt"
	"sync"
	"time"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	"github.com/Microsoft/hcsshim/internal/controller/process"
	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/signals"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"

	"github.com/Microsoft/go-winio/pkg/guid"
	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/api/runtime/task/v2"
	containerdtypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/errdefs"
	"github.com/containerd/typeurl/v2"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Controller is the concrete implementation of the LCOW container controller.
// It manages the full lifecycle of a single LCOW container.
type Controller struct {
	// mu guards all mutable fields in this struct.
	mu sync.RWMutex

	// vmID is the identifier of the utility VM that hosts this container.
	vmID string

	// gcsPodID is the sandbox/pod identifier within the GCS.
	gcsPodID string

	// containerID is the unique identifier for this container.
	// This is the containerd-visible identifier.
	containerID string

	// gcsContainerID is the identifier for the container used
	// while interacting with GCS.
	gcsContainerID string

	// guest is used to create and manage the GCS container entity.
	guest guest

	// scsi manages SCSI disk attachments for the container.
	scsi scsiController

	// plan9 manages Plan9 file-share mounts for the container.
	plan9 plan9Controller

	// vpci manages virtual PCI device assignments for the container.
	vpci vPCIController

	// Host-side resource reservations released during teardown.
	layers         *scsiLayers
	scsiResources  []guid.GUID
	plan9Resources []guid.GUID
	devices        []guid.GUID

	// container is the GCS container handle used for lifecycle operations.
	container *gcs.Container

	// state tracks the current lifecycle state of the container.
	// Access must be guarded by mu.
	state State

	// terminatedCh is closed exactly once when the container is closed.
	// All callers of Wait block on this channel, and closing it unblocks
	// every waiter simultaneously.
	terminatedCh chan struct{}

	// processes maps exec IDs to their process controllers.
	// The init process is stored with exec ID "".
	// Access must be guarded by mu.
	processes map[string]*process.Controller

	// ioRetryTimeout is the duration to retry IO relay operations before giving up.
	ioRetryTimeout time.Duration
}

// New creates a ready-to-use Controller.
func New(
	vmID string,
	gcsPodID string,
	containerID string,
	guestMgr guest,
	scsiCtrl scsiController,
	plan9Ctrl plan9Controller,
	vpci vPCIController,
) *Controller {
	return &Controller{
		vmID:        vmID,
		gcsPodID:    gcsPodID,
		containerID: containerID,
		// Same id is used as the container. Post migration, we can always
		// change the primary ID while gcs uses the original ID.
		gcsContainerID: containerID,
		guest:          guestMgr,
		scsi:           scsiCtrl,
		plan9:          plan9Ctrl,
		vpci:           vpci,
		processes:      make(map[string]*process.Controller),
		state:          StateNotCreated,
		terminatedCh:   make(chan struct{}),
	}
}

// Create allocates host-side resources, creates the container in the guest,
// and sets up the init process.
func (c *Controller) Create(ctx context.Context, spec *specs.Spec, opts *task.CreateTaskRequest, copts *CreateOpts) (err error) {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.GCSContainerID, c.gcsContainerID))
	log.G(ctx).Debug("creating container")

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateNotCreated {
		return fmt.Errorf("container %s is in state %s; cannot create: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	// Parse the runtime options from the request.
	shimOpts, err := vmutils.UnmarshalRuntimeOptions(ctx, opts.Options)
	if err != nil {
		return fmt.Errorf("unmarshal runtime options: %w", err)
	}

	// Apply any updates to the OCI spec based on the shim options.
	*spec = oci.UpdateSpecFromOptions(*spec, shimOpts)

	// Expand annotations after defaults have been loaded in from options.
	// Since annotation expansion is used to toggle security features,
	// raise the error rather than suppress and move on.
	if err = oci.ProcessAnnotations(ctx, spec.Annotations); err != nil {
		return fmt.Errorf("process OCI spec annotations: %w", err)
	}

	// Upon any failure from this point onwards, perform a teardown
	// of container and set state as invalid.
	defer func() {
		if err != nil {
			c.state = StateInvalid
			// If we fail during create, then there won't be an opportunity to
			// call Delete and therefore, we need to perform the best effort cleanup here.
			if releaseErr := c.releaseResources(ctx); releaseErr != nil {
				log.G(ctx).WithError(releaseErr).Error("failed to release resources during create")
			}
			if closeErr := c.closeContainer(ctx); closeErr != nil {
				log.G(ctx).WithError(closeErr).Error("failed to close container during create")
			}
		}
	}()

	// Allocate all host-side resources and build the GCS container document.
	gcsDocument, err := c.generateContainerDocument(ctx, spec, opts.Rootfs, copts.IsScratchEncryptionEnabled)
	if err != nil {
		return fmt.Errorf("generate container document: %w", err)
	}

	// Create the container within the UVM.
	c.container, err = c.guest.CreateContainer(ctx, c.gcsContainerID, gcsDocument)
	if err != nil {
		return fmt.Errorf("create container in guest: %w", err)
	}

	// Default to an infinite timeout (zero value).
	if shimOpts != nil {
		c.ioRetryTimeout = time.Duration(shimOpts.IoRetryTimeoutInSec) * time.Second
	}

	// Create the initial process controller with exec ID "".
	initProcess := process.New(c.containerID, "", c.container, c.ioRetryTimeout)
	if err = initProcess.Create(ctx, &process.CreateOptions{
		Bundle:   opts.Bundle,
		Terminal: opts.Terminal,
		Stdin:    opts.Stdin,
		Stdout:   opts.Stdout,
		Stderr:   opts.Stderr,
	}); err != nil {
		return fmt.Errorf("create init process: %w", err)
	}
	c.processes[""] = initProcess

	c.state = StateCreated
	return nil
}

// closeContainer performs container teardown. It is safe to retry on
// failure. Needs to be called while holding c.mu lock.
func (c *Controller) closeContainer(ctx context.Context) error {
	if c.container != nil {
		// Delete the guest-side container state if supported. If this
		// fails, return early without nil'ing c.container so a retry
		// re-issues the request.
		if c.guest.Capabilities().IsDeleteContainerStateSupported() {
			if err := c.guest.DeleteContainerState(ctx, c.gcsContainerID); err != nil {
				return fmt.Errorf("delete container state: %w", err)
			}
		}

		// Close the container handle. The calling code never returns error.
		_ = c.container.Close()
		c.container = nil
	}

	// Release all waiters exactly once. A non-blocking receive distinguishes
	// an already-closed channel from one that still needs closing.
	select {
	case <-c.terminatedCh:
		// already closed
	default:
		close(c.terminatedCh)
	}
	return nil
}

// releaseResources undoes each allocation in reverse order.
// It is idempotent — subsequent calls after the first are no-ops.
func (c *Controller) releaseResources(ctx context.Context) error {
	// Combined layers must be removed before unmapping the underlying SCSI
	// layer devices.
	if c.layers != nil && c.layers.layersCombined {
		hcsLayers := make([]hcsschema.Layer, 0, len(c.layers.roLayers))
		for _, layer := range c.layers.roLayers {
			hcsLayers = append(hcsLayers, hcsschema.Layer{Path: layer.guestPath})
		}

		if err := c.guest.RemoveCombinedLayers(ctx, guestresource.LCOWCombinedLayers{
			ContainerID:       c.gcsContainerID,
			ContainerRootPath: c.layers.rootfsPath,
			Layers:            hcsLayers,
			ScratchPath:       c.layers.scratch.guestPath,
		}); err != nil {
			return fmt.Errorf("remove combined layers from guest: %w", err)
		}

		// Set layersCombined to false so that we do not retry this post successful remove.
		c.layers.layersCombined = false
	}

	// Unmap the scratch layer. A zero ID indicates it has already been
	// unmapped on a prior call.
	var zeroGUID guid.GUID
	if c.layers != nil && c.layers.scratch.id != zeroGUID {
		if err := c.scsi.UnmapFromGuest(ctx, c.layers.scratch.id); err != nil {
			return fmt.Errorf("unmap scratch layer: %w", err)
		}
		c.layers.scratch = scsiReservation{}
	}

	// Unmap RO layers. On failure, retain the unprocessed tail so a retry
	// resumes from the first failure.
	if c.layers != nil {
		for i, layer := range c.layers.roLayers {
			if err := c.scsi.UnmapFromGuest(ctx, layer.id); err != nil {
				c.layers.roLayers = c.layers.roLayers[i:]
				return fmt.Errorf("unmap ro layer: %w", err)
			}
		}
	}

	// Unmap additional SCSI mounts.
	for i, id := range c.scsiResources {
		if err := c.scsi.UnmapFromGuest(ctx, id); err != nil {
			c.scsiResources = c.scsiResources[i:]
			return fmt.Errorf("unmap scsi resource: %w", err)
		}
	}

	// Unmap Plan9 shares.
	for i, id := range c.plan9Resources {
		if err := c.plan9.UnmapFromGuest(ctx, id); err != nil {
			c.plan9Resources = c.plan9Resources[i:]
			return fmt.Errorf("unmap plan9 share: %w", err)
		}
	}

	// Remove VPCI devices.
	for i, id := range c.devices {
		if err := c.vpci.RemoveFromVM(ctx, id); err != nil {
			c.devices = c.devices[i:]
			return fmt.Errorf("remove vpci device: %w", err)
		}
	}

	return nil
}

// Start starts the container and its init process, returning the init PID.
func (c *Controller) Start(ctx context.Context, events chan interface{}) (uint32, error) {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.GCSContainerID, c.gcsContainerID))
	log.G(ctx).Debug("starting container")

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateCreated {
		return 1, fmt.Errorf("container %s is in state %s; cannot start: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	// Start the container.
	if err := c.container.Start(ctx); err != nil {
		c.state = StateInvalid
		return 1, fmt.Errorf("start container %s: %w", c.containerID, err)
	}

	// Start the init process. Pass nil for sendEvent because the init
	// process exit event is published by handleInitProcessExit after
	// full container teardown.
	initProcess := c.processes[""]
	pid, err := initProcess.Start(ctx, nil)
	if err != nil {
		c.state = StateInvalid
		return 1, fmt.Errorf("start init process: %w", err)
	}

	c.state = StateRunning
	go c.handleInitProcessExit(ctx, initProcess, events)

	return uint32(pid), nil
}

// handleInitProcessExit blocks until the init process exits, then tears down
// the container, marks it stopped, and publishes the exit event.
func (c *Controller) handleInitProcessExit(ctx context.Context, initProcess *process.Controller, events chan interface{}) {
	// Detach from the caller's context so upstream cancellation/timeout does
	// not abort the background teardown.
	ctx = context.WithoutCancel(ctx)

	// Block until the init process exits.
	initProcess.Wait(ctx)

	c.mu.Lock()
	c.state = StateStopped
	if err := c.closeContainer(ctx); err != nil {
		// Leave state as StateStopped so DeleteProcess can retry the
		// teardown. The exit event below still informs the caller that
		// the init process is gone.
		log.G(ctx).WithError(err).Error("failed to close container after init exit")
	}
	c.mu.Unlock()

	// Publish the exit event after teardown is complete.
	if events != nil {
		status := initProcess.Status(true)
		events <- &eventstypes.TaskExit{
			ContainerID: c.containerID,
			ID:          status.ExecID,
			Pid:         status.Pid,
			ExitStatus:  status.ExitStatus,
			ExitedAt:    status.ExitedAt,
		}
	}
}

// Wait blocks until the container has fully terminated.
func (c *Controller) Wait(ctx context.Context) {
	select {
	case <-c.terminatedCh:
	case <-ctx.Done():
		log.G(ctx).WithError(ctx.Err()).Error("wait for container to exit failed")
	}
}

// Update modifies the container's resource constraints.
func (c *Controller) Update(ctx context.Context, resources interface{}) error {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.GCSContainerID, c.gcsContainerID))
	log.G(ctx).Debug("updating container")

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateRunning {
		return fmt.Errorf("container %s is in state %s; cannot update: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	linuxRes, ok := resources.(*specs.LinuxResources)
	if !ok {
		return fmt.Errorf("invalid container resources: expected *specs.LinuxResources, got %T", resources)
	}

	return c.container.Modify(ctx, guestrequest.ModificationRequest{
		ResourceType: guestresource.ResourceTypeContainerConstraints,
		RequestType:  guestrequest.RequestTypeUpdate,
		Settings: guestresource.LCOWContainerConstraints{
			Linux: *linuxRes,
		},
	})
}

// NewProcess creates a new exec process controller in the container.
func (c *Controller) NewProcess(execID string) (*process.Controller, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateRunning {
		return nil, fmt.Errorf("container %s is in state %s; cannot create new process: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	if _, exists := c.processes[execID]; exists {
		return nil, fmt.Errorf("exec process %q already exists in container %s", execID, c.containerID)
	}

	newProcess := process.New(c.containerID, execID, c.container, c.ioRetryTimeout)
	c.processes[execID] = newProcess

	return newProcess, nil
}

// GetProcess returns the process controller for the given exec ID.
func (c *Controller) GetProcess(execID string) (*process.Controller, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.getProcess(execID)
}

// getProcess returns the process controller for the given exec ID.
// The caller must hold c.mu (for reading or writing).
func (c *Controller) getProcess(execID string) (*process.Controller, error) {
	proc, ok := c.processes[execID]
	if !ok {
		return nil, fmt.Errorf("process %q not found in container %s: %w",
			execID, c.containerID, errdefs.ErrNotFound)
	}
	return proc, nil
}

// ListProcesses returns all exec processes (excluding the init process).
func (c *Controller) ListProcesses() (map[string]*process.Controller, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*process.Controller, len(c.processes))
	for id, proc := range c.processes {
		if id == "" {
			continue
		}
		result[id] = proc
	}
	return result, nil
}

// Pids queries the guest for the full process list and annotates each entry
// with the exec ID from the local process registry.
func (c *Controller) Pids(ctx context.Context) ([]*containerdtypes.ProcessInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.state != StateRunning {
		return nil, fmt.Errorf("container %s is in state %s; cannot query pids: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	// Build a pid→execID lookup from locally tracked processes.
	pidMap := make(map[int]string, len(c.processes))
	for execID, proc := range c.processes {
		pidMap[proc.Pid()] = execID
	}

	// Query the guest for the actual process list.
	props, err := c.container.Properties(ctx, schema1.PropertyTypeProcessList)
	if err != nil {
		return nil, fmt.Errorf("fetch container properties: %w", err)
	}

	// Build ProcessDetails for each process in the guest.
	processes := make([]*containerdtypes.ProcessInfo, len(props.ProcessList))
	for i, proc := range props.ProcessList {
		pd := &runhcsopts.ProcessDetails{
			ImageName:                    proc.ImageName,
			CreatedAt:                    timestamppb.New(proc.CreateTimestamp),
			KernelTime_100Ns:             proc.KernelTime100ns,
			MemoryCommitBytes:            proc.MemoryCommitBytes,
			MemoryWorkingSetPrivateBytes: proc.MemoryWorkingSetPrivateBytes,
			MemoryWorkingSetSharedBytes:  proc.MemoryWorkingSetSharedBytes,
			ProcessID:                    proc.ProcessId,
			UserTime_100Ns:               proc.UserTime100ns,
		}
		if execID, ok := pidMap[int(proc.ProcessId)]; ok {
			pd.ExecID = execID
		}

		anyVal, err := typeurl.MarshalAny(pd)
		if err != nil {
			return nil, fmt.Errorf("marshal process details for exec %s in container %s: %w", pd.ExecID, c.containerID, err)
		}
		processes[i] = &containerdtypes.ProcessInfo{
			Pid:  pd.ProcessID,
			Info: typeurl.MarshalProto(anyVal),
		}
	}
	return processes, nil
}

// Stats returns the runtime statistics for the container.
func (c *Controller) Stats(ctx context.Context) (*stats.Statistics, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.state != StateRunning {
		return nil, fmt.Errorf("container %s is in state %s; cannot fetch stats: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	props, err := c.container.PropertiesV2(ctx, hcsschema.PTStatistics)
	if err != nil {
		return nil, fmt.Errorf("fetch container statistics: %w", err)
	}

	containerStats := &stats.Statistics{}
	if props != nil {
		containerStats.Container = &stats.Statistics_Linux{Linux: props.Metrics}
	}
	return containerStats, nil
}

// KillProcess delivers a signal to the specified process or all processes in the container.
func (c *Controller) KillProcess(ctx context.Context, execID string, signal uint32, all bool) error {
	if all && execID != "" {
		return fmt.Errorf("cannot signal all for non-empty exec %q: %w", execID, errdefs.ErrFailedPrecondition)
	}

	signalsSupported := c.guest.Capabilities().IsSignalProcessSupported()
	signalOptions, err := signals.ValidateLCOW(int(signal), signalsSupported)
	if err != nil {
		return fmt.Errorf("validate signal %d for container %s: %w", signal, c.containerID, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// The container must have been created for any process to exist.
	if c.state == StateNotCreated {
		return fmt.Errorf("container %s is in state %s; cannot kill: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	// When "all" is requested, deliver the signal to every additional exec
	// on a best-effort basis. Errors are logged but do not prevent the
	// target process from being signaled.
	if all {
		for eid, proc := range c.processes {
			if eid == "" {
				// The init process is signaled as the explicit target below.
				continue
			}
			if killErr := proc.Kill(ctx, signalOptions); killErr != nil {
				log.G(ctx).WithError(killErr).WithField(logfields.ExecID, eid).Warn("failed to kill exec in container")
			}
		}
	}

	// Now signal the actual process identified by execID.
	targetProcess, err := c.getProcess(execID)
	if err != nil {
		return err
	}
	return targetProcess.Kill(ctx, signalOptions)
}

// DeleteProcess removes the process identified by execID and returns its last status.
func (c *Controller) DeleteProcess(ctx context.Context, execID string) (*task.StateResponse, error) {
	// When deleting the init process, wait for handleInitProcessExit to
	// complete container teardown first.
	// In short, this prevents race of DeleteProcess with handleInitProcessExit.
	if execID == "" {
		c.mu.RLock()
		isStarted := c.state == StateRunning || c.state == StateStopped
		c.mu.RUnlock()

		if isStarted {
			waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			c.Wait(waitCtx)
			if waitCtx.Err() != nil {
				return nil, fmt.Errorf("wait for container %s resource cleanup: %w", c.containerID, waitCtx.Err())
			}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// The container must have been created for any process to exist.
	if c.state == StateNotCreated {
		return nil, fmt.Errorf("container %s is in state %s; cannot delete process: %w", c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	proc, err := c.getProcess(execID)
	if err != nil {
		return nil, err
	}

	// Move the process into deleted state.
	if err = proc.Delete(ctx); err != nil {
		return nil, err
	}

	// Capture the process status before removing the entry from map.
	status := proc.Status(true)

	// Deleting the init process (execID "") means the container itself is
	// being torn down.
	if execID == "" {
		// For containers that were created but never started, handleInitProcessExit
		// was never launched, so closeContainer was never called. Perform full
		// teardown now. closeContainer is retriable.
		if err = c.closeContainer(ctx); err != nil {
			return nil, fmt.Errorf("close container %s: %w", c.containerID, err)
		}
		if err = c.releaseResources(ctx); err != nil {
			return nil, fmt.Errorf("releasing resources for container %s: %w", c.containerID, err)
		}
	}

	// Remove the process entry only after all fallible operations have
	// succeeded, so that a retry can still locate the process.
	delete(c.processes, execID)

	return status, nil
}
