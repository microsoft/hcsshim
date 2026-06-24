//go:build windows && lcow

package pod

import (
	"context"
	"fmt"
	"sort"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi"
	"github.com/Microsoft/hcsshim/internal/controller/linuxcontainer"
	"github.com/Microsoft/hcsshim/internal/controller/network"
	podsave "github.com/Microsoft/hcsshim/internal/controller/pod/save"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/containerd/containerd/api/runtime/task/v3"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// Save returns a serialized snapshot of the pod — its identifiers plus the
// network and per-container state — wrapped in an [anypb.Any] for the caller
// to ship to a migration destination. After it returns, all operations are
// rejected until migration is resumed.
func (c *Controller) Save(ctx context.Context) (*anypb.Any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Snapshot containers in a fixed order so the same pod always serializes
	// to the same bytes, which keeps snapshot diffs and tests stable.
	ids := make([]string, 0, len(c.containers))
	for id := range c.containers {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// Serialize each container into its own opaque envelope.
	containers := make([]*anypb.Any, 0, len(ids))
	for _, id := range ids {
		cs, err := c.containers[id].Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("save container %q: %w", id, err)
		}
		containers = append(containers, cs)
	}

	// Assemble the pod-level payload with its identifiers and children.
	state := &podsave.Payload{
		SchemaVersion: podsave.SchemaVersion,
		PodID:         c.podID,
		GcsPodID:      c.gcsPodID,
		Containers:    containers,
	}

	// Fold in the network snapshot.
	if c.network != nil {
		ns, err := c.network.Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("save network controller: %w", err)
		}
		state.Network = ns
	}

	// Marshal the assembled payload into the typed migration envelope.
	payload, err := proto.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal pod saved state for %q: %w", c.podID, err)
	}

	// Block all further operations until migration is resumed.
	c.isMigrating = true

	log.G(ctx).WithField(logfields.SourcePodID, c.podID).Debug("saved pod state")

	return &anypb.Any{TypeUrl: podsave.TypeURL, Value: payload}, nil
}

// Import reconstructs a pod [Controller] from a [Controller.Save] envelope.
// The returned controller is inert: its network and containers are restored
// but not bound to a live VM, so it does nothing until [Controller.Resume].
func Import(ctx context.Context, env *anypb.Any) (*Controller, error) {
	// Reject an empty or mistyped envelope before touching its bytes.
	if env == nil {
		return nil, fmt.Errorf("pod saved-state envelope is nil")
	}

	if env.GetTypeUrl() != podsave.TypeURL {
		return nil, fmt.Errorf("unsupported pod saved-state type %q", env.GetTypeUrl())
	}

	// Decode and reject any payload this build cannot interpret.
	state := &podsave.Payload{}
	if err := proto.Unmarshal(env.GetValue(), state); err != nil {
		return nil, fmt.Errorf("unmarshal pod saved state: %w", err)
	}

	if v := state.GetSchemaVersion(); v != podsave.SchemaVersion {
		return nil, fmt.Errorf("unsupported pod saved-state schema version %d (want %d)", v, podsave.SchemaVersion)
	}

	// Restore the network controller in its own inert state.
	netCtrl, err := network.Import(ctx, state.GetNetwork())
	if err != nil {
		return nil, fmt.Errorf("import network controller: %w", err)
	}

	// Rebuild the pod shell with its identifiers and the restored network.
	c := &Controller{
		podID:       state.GetPodID(),
		gcsPodID:    state.GetGcsPodID(),
		containers:  make(map[string]*linuxcontainer.Controller, len(state.GetContainers())),
		network:     netCtrl,
		isMigrating: true,
	}

	// Rehydrate each container and re-key by its own restored ID.
	for _, cAny := range state.GetContainers() {
		ctr, err := linuxcontainer.Import(ctx, cAny)
		if err != nil {
			return nil, fmt.Errorf("import container in pod %q: %w", c.podID, err)
		}
		c.containers[ctr.ContainerID()] = ctr
	}

	log.G(ctx).WithField(logfields.SourcePodID, c.podID).Debug("imported pod state")

	return c, nil
}

// Resume brings a migrating pod back online by binding the live VM, reattaching
// the network, and resuming every container. Only the destination
// (isDestination) swaps the source NIC bindings for its own namespace; the
// source keeps the network it already has.
//
// Must be called only after the VM's GCS bridge is up, since each container
// reopens its guest container over that bridge. On the destination each
// container republishes a TaskCreate on events upon resume.
func (c *Controller) Resume(ctx context.Context, vm vmController, events chan interface{}, isDestination bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Bind the live VM and re-wire the network to it.
	c.vm = vm
	c.isMigrating = false
	c.network.Resume(ctx, vm.VM(), vm.Guest())

	// Fetch the SCSI controller shared by all containers in this VM.
	scsiCtrl, err := vm.SCSIController(ctx)
	if err != nil {
		return fmt.Errorf("get SCSI controller: %w", err)
	}

	// Resume each container against the live guest and device controllers.
	for _, ctr := range c.containers {
		if err := ctr.Resume(
			ctx,
			vm.RuntimeID(),
			c.gcsPodID,
			vm.Guest(),
			scsiCtrl,
			vm.Plan9Controller(),
			vm.VPCIController(),
			events,
		); err != nil {
			return fmt.Errorf("resume container %q: %w", ctr.ContainerID(), err)
		}
	}

	// Only the destination rebinds networking to its own namespace; the source
	// keeps the network it already has.
	if isDestination {
		if err := c.network.ResetAfterMigration(ctx); err != nil {
			return fmt.Errorf("reset network for migration: %w", err)
		}
	}

	log.G(ctx).WithField(logfields.DestinationPodID, c.podID).Debug("resumed pod")

	return nil
}

// Patch updates a migrated container so it matches the new task created by
// containerd on this destination host. It points the container at the
// destination's disk paths and assigns the new container ID and IO carried in
// request. For the sandbox container it also takes on the new pod ID and
// records the destination network namespace for a later attach.
func (c *Controller) Patch(
	ctx context.Context,
	sourceContainerID string,
	isSandbox bool,
	scsiCtrl *scsi.Controller,
	request *task.CreateTaskRequest,
	spec specs.Spec,
) error {
	// A destination request with a container ID is required before we mutate
	// any state.
	if request == nil || request.ID == "" {
		return fmt.Errorf("invalid create task request: %+v", request)
	}

	log.G(ctx).WithFields(logrus.Fields{
		logfields.SourcePodID:      c.podID,
		logfields.DestinationPodID: request.ID,
		"IsSandbox":                isSandbox,
		"Spec":                     log.Format(ctx, spec),
	}).Debug("patching pod")

	c.mu.Lock()
	defer c.mu.Unlock()

	// Resolve the container by its source-side ID and guard against colliding
	// with an existing container on destination when the ID is changing.
	ctr, ok := c.containers[sourceContainerID]
	if !ok {
		return fmt.Errorf("source container %q not found in pod %q", sourceContainerID, c.podID)
	}

	// If the ID is changing, reject the rename when a different container
	// already occupies the destination ID.
	if _, exists := c.containers[request.ID]; exists && sourceContainerID != request.ID {
		return fmt.Errorf("destination container %q already exists in pod %q", request.ID, c.podID)
	}

	// Retarget the container's identity and resource paths to the destination.
	if err := ctr.Patch(ctx, scsiCtrl, request); err != nil {
		return fmt.Errorf("patch source container %q: %w", sourceContainerID, err)
	}

	// Re-key the container under its new ID once the patch succeeds.
	if sourceContainerID != request.ID {
		delete(c.containers, sourceContainerID)
		c.containers[request.ID] = ctr
	}

	// A sandbox container also carries the pod identity and network namespace.
	if isSandbox {
		// Adopt the destination pod ID.
		c.podID = request.ID

		// The sandbox spec must name the destination network namespace.
		if spec.Windows == nil || spec.Windows.Network == nil || spec.Windows.Network.NetworkNamespace == "" {
			return fmt.Errorf("windows network namespace is required for sandbox container")
		}

		// Hand the destination namespace to the network controller for later attach.
		c.network.Patch(ctx, spec.Windows.Network.NetworkNamespace)
	}

	log.G(ctx).WithField(logfields.DestinationPodID, c.podID).Debug("patched migrated pod")
	return nil
}

// AbortMigrated marks every container in the pod as stopped and reports their
// exit, so that containerd no longer sees them as UNKNOWN and can delete them.
// This is primarily used on destination during finalize stop.
func (c *Controller) AbortMigrated(ctx context.Context, events chan interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, ctr := range c.containers {
		ctr.AbortMigrated(ctx, events)
	}
}
