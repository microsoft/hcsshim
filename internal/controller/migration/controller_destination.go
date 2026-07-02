//go:build windows && lcow

package migration

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	save "github.com/Microsoft/hcsshim/internal/controller/migration/save"
	"github.com/Microsoft/hcsshim/internal/controller/pod"
	"github.com/Microsoft/hcsshim/internal/controller/vm"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oci"
	hcsannotations "github.com/Microsoft/hcsshim/pkg/annotations"

	"github.com/containerd/containerd/api/runtime/task/v3"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// ImportState rehydrates a source-side migration snapshot onto this controller.
// A repeat call for the same session is a no-op.
func (c *Controller) ImportState(ctx context.Context, opts *ImportStateOptions) error {
	// Reject malformed input up front so the controller is never mutated
	// on the basis of a half-specified request.
	switch {
	case opts == nil:
		return fmt.Errorf("options are required: %w", errdefs.ErrInvalidArgument)
	case opts.SessionID == "":
		return fmt.Errorf("session id is required: %w", errdefs.ErrInvalidArgument)
	case opts.VMController == nil:
		return fmt.Errorf("vm controller is required: %w", errdefs.ErrInvalidArgument)
	case opts.SandboxID == "":
		return fmt.Errorf("sandbox id is required: %w", errdefs.ErrInvalidArgument)
	case opts.PodControllers == nil:
		return fmt.Errorf("pod controllers map is required: %w", errdefs.ErrInvalidArgument)
	case opts.ContainerPodMapping == nil:
		return fmt.Errorf("container-pod mapping is required: %w", errdefs.ErrInvalidArgument)
	case opts.SavedState == nil:
		return fmt.Errorf("sandbox saved state is required: %w", errdefs.ErrInvalidArgument)
	case opts.SavedState.TypeUrl != save.TypeURL:
		return fmt.Errorf("unsupported sandbox saved-state type %q: %w", opts.SavedState.TypeUrl, errdefs.ErrInvalidArgument)
	case opts.VMController.State() != vm.StateNotCreated:
		return fmt.Errorf("vm controller is in invalid state %s: %w", opts.VMController.State(), errdefs.ErrFailedPrecondition)
	default:
	}

	decoded := &save.Payload{}
	if err := proto.Unmarshal(opts.SavedState.Value, decoded); err != nil {
		return fmt.Errorf("unmarshal sandbox saved state: %w", err)
	}
	if decoded.GetSchemaVersion() != save.SchemaVersion {
		return fmt.Errorf("sandbox saved-state schema version %d not supported (want %d): %w",
			decoded.GetSchemaVersion(), save.SchemaVersion, errdefs.ErrInvalidArgument)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Idempotent retry for the same session; any other non-idle state is a conflict.
	if c.state == StateDestinationImported && c.sessionID == opts.SessionID {
		return nil
	}
	if c.state != StateIdle {
		return fmt.Errorf("controller is in state %s for session %q: %w", c.state, c.sessionID, errdefs.ErrAlreadyExists)
	}

	// Rehydrate the VM controller from the saved VM payload.
	if err := opts.VMController.Import(ctx, decoded.GetVm()); err != nil {
		return fmt.Errorf("import vm controller: %w", err)
	}

	// Rehydrate each pod and index its containers so PatchResourcePaths
	// can look up the owning pod.
	pending := make(map[string]struct{})
	for _, podAny := range decoded.GetPods() {
		// Rebuild the pod controller from its saved payload.
		importedPod, err := pod.Import(ctx, podAny)
		if err != nil {
			return fmt.Errorf("import pod: %w", err)
		}

		opts.PodControllers[importedPod.PodID()] = importedPod

		// Source container IDs must be unique across pods so the later patch
		// lookup is unambiguous.
		for containerID := range importedPod.ListContainers() {
			if _, dup := pending[containerID]; dup {
				return fmt.Errorf("duplicate source container id %q across imported pods: %w", containerID, errdefs.ErrInvalidArgument)
			}

			// Track the container as awaiting a patch and map it to its pod.
			pending[containerID] = struct{}{}
			opts.ContainerPodMapping[containerID] = importedPod.PodID()
		}
	}

	c.sessionID = opts.SessionID
	c.sandboxID = opts.SandboxID
	c.origin = opts.Origin
	c.vmController = opts.VMController
	c.podControllers = opts.PodControllers
	c.containerPodMapping = opts.ContainerPodMapping
	c.pendingPatches = pending
	c.state = StateDestinationImported

	log.G(ctx).Info("migration destination state imported")
	return nil
}

// PatchResourcePaths rewrites the imported source container's identifiers to
// the destination IDs carried by request and spec. A container may be patched
// only once; a repeat call for an already-patched container fails.
func (c *Controller) PatchResourcePaths(
	ctx context.Context,
	request *task.CreateTaskRequest,
	spec specs.Spec,
) error {
	switch {
	case request == nil:
		return fmt.Errorf("request is required: %w", errdefs.ErrInvalidArgument)
	case request.ID == "":
		return fmt.Errorf("destination container id is required: %w", errdefs.ErrInvalidArgument)
	case spec.Annotations == nil:
		return fmt.Errorf("annotations are required: %w", errdefs.ErrInvalidArgument)
	}

	// The source container being rebound is identified by the annotation.
	sourceContainerID := spec.Annotations[hcsannotations.LiveMigrationSourceContainerID]
	if sourceContainerID == "" {
		return fmt.Errorf("annotation %q is required: %w", hcsannotations.LiveMigrationSourceContainerID, errdefs.ErrInvalidArgument)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateDestinationImported {
		return fmt.Errorf("patch not valid in state %s: %w", c.state, errdefs.ErrFailedPrecondition)
	}

	// A container may be patched only once; once patched, it drops out of
	// pendingPatches. Hence, a non-pending ID has already been patched.
	if _, pending := c.pendingPatches[sourceContainerID]; !pending {
		return fmt.Errorf("source container %q is not pending a patch: %w", sourceContainerID, errdefs.ErrAlreadyExists)
	}

	// containerPodMapping is the source-of-truth index built in ImportState
	// and rewritten in place here, so a direct lookup beats scanning pods.
	sourcePodID, ok := c.containerPodMapping[sourceContainerID]
	if !ok {
		return fmt.Errorf("source container %q not found: %w", sourceContainerID, errdefs.ErrNotFound)
	}
	podCtrl := c.podControllers[sourcePodID]

	// Sandbox is detected structurally as container ID == pod ID.
	isSandbox := sourceContainerID == sourcePodID

	// The K8s container-type annotation, if present, must agree with the
	// structural detection; a mismatch would desync the in-pod and outer-map renames.
	if v := spec.Annotations[hcsannotations.KubernetesContainerType]; v != "" {
		annotationSaysSandbox := v == string(oci.KubernetesContainerTypeSandbox)
		if annotationSaysSandbox != isSandbox {
			return fmt.Errorf("annotation %q=%q disagrees with structural sandbox detection (sourceContainerID=%q, sourcePodID=%q): %w",
				hcsannotations.KubernetesContainerType, v, sourceContainerID, sourcePodID, errdefs.ErrInvalidArgument)
		}
	}

	// Fetch the SCSI controller so that we can patch the VHD paths on destination.
	scsiCtrl, err := c.vmController.SCSIController(ctx)
	if err != nil {
		return fmt.Errorf("get scsi controller for patch: %w", err)
	}

	// Rebind the container's resources within its pod to the destination IDs.
	if err := podCtrl.Patch(ctx, sourceContainerID, isSandbox, scsiCtrl, request, spec); err != nil {
		return fmt.Errorf("patch source container %q in pod %q: %w", sourceContainerID, sourcePodID, err)
	}

	// Patching the sandbox renames the pod: re-key its podControllers entry and
	// repoint every container still mapped to the old pod ID at the new one.
	if isSandbox {
		// Move the pod controller from the old pod ID to the destination ID.
		delete(c.podControllers, sourcePodID)
		c.podControllers[request.ID] = podCtrl

		// Repoint any container (this sandbox plus worker containers patched earlier)
		// that still references the old pod ID at the new pod ID.
		for cid, pid := range c.containerPodMapping {
			if pid == sourcePodID {
				c.containerPodMapping[cid] = request.ID
			}
		}
	}

	// Re-key this container's mapping entry from its source to destination ID.
	delete(c.containerPodMapping, sourceContainerID)
	c.containerPodMapping[request.ID] = podCtrl.PodID()

	// The container is now patched, so drop it from the pending set.
	delete(c.pendingPatches, sourceContainerID)

	log.G(ctx).WithFields(logrus.Fields{
		logfields.SourceContainerID:      sourceContainerID,
		logfields.DestinationContainerID: request.ID,
	}).Info("migration container resource paths patched")

	return nil
}

// PrepareDestination materialises the destination HCS compute system once every
// imported container has been patched to its destination identifiers.
func (c *Controller) PrepareDestination(ctx context.Context, sessionID string, migrationOpts *hcsschema.MigrationInitializeOptions) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sessionID != sessionID {
		return fmt.Errorf("session id %q does not match active session %q: %w", sessionID, c.sessionID, errdefs.ErrInvalidArgument)
	}

	if c.state != StateDestinationImported {
		return fmt.Errorf("prepare destination not valid in state %s: %w", c.state, errdefs.ErrFailedPrecondition)
	}

	// Every imported container must be patched before the destination VM is built.
	if len(c.pendingPatches) > 0 {
		pending := slices.Sorted(maps.Keys(c.pendingPatches))
		return fmt.Errorf("%d container patches still pending [%s]: %w",
			len(pending), strings.Join(pending, ", "), errdefs.ErrFailedPrecondition)
	}

	// Default options and stamp the destination origin so HCS sees a
	// complete config regardless of caller input.
	if migrationOpts == nil {
		migrationOpts = &hcsschema.MigrationInitializeOptions{}
	}
	migrationOpts.Origin = c.origin

	// Build the destination VM's HCS compute system from the imported config.
	if err := c.vmController.CreateVM(ctx,
		&vm.CreateOptions{
			ID:               fmt.Sprintf("%s@vm", c.sandboxID),
			MigrationOptions: migrationOpts,
		}); err != nil {
		return fmt.Errorf("create destination vm: %w", err)
	}

	// Re-ACL the patched (destination-host) VHDs against the freshly
	// created VM's SID so it can open them once it starts.
	if err := c.vmController.Patch(ctx); err != nil {
		return fmt.Errorf("patch destination vm: %w", err)
	}

	c.state = StateDestinationPrepared
	log.G(ctx).Info("migration destination prepared")
	return nil
}
