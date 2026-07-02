//go:build windows && lcow

package migration

import (
	"context"
	"fmt"

	save "github.com/Microsoft/hcsshim/internal/controller/migration/save"
	"github.com/Microsoft/hcsshim/internal/controller/vm"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"

	"github.com/containerd/errdefs"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// PrepareSource readies the source side of a migration session so a subsequent
// [Controller.ExportState] can capture its state. A repeat call for the same
// session is a no-op.
func (c *Controller) PrepareSource(ctx context.Context, opts *PrepareSourceOptions) error {
	switch {
	case opts == nil:
		return fmt.Errorf("options are required: %w", errdefs.ErrInvalidArgument)
	case opts.SessionID == "":
		return fmt.Errorf("session id is required: %w", errdefs.ErrInvalidArgument)
	case opts.VMController == nil:
		return fmt.Errorf("vm controller is required: %w", errdefs.ErrInvalidArgument)
	case opts.PodControllers == nil:
		return fmt.Errorf("pod controllers map is required: %w", errdefs.ErrInvalidArgument)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateIdle {
		// If we already called this API, then this is a no-op.
		if c.state == StateSourcePrepared && c.sessionID == opts.SessionID {
			return nil
		}
		return fmt.Errorf("controller is in state %s for session %q: %w", c.state, c.sessionID, errdefs.ErrFailedPrecondition)
	}

	// VM must be running in order to initiate the migration.
	if opts.VMController.State() != vm.StateRunning {
		return fmt.Errorf("vm controller is in invalid state %s: %w", opts.VMController.State(), errdefs.ErrFailedPrecondition)
	}

	// The sandbox must have been created with live migration enabled.
	// Reject otherwise so callers learn the session cannot proceed before
	// any source-side state is mutated.
	sandboxOpts := opts.VMController.SandboxOptions()
	if sandboxOpts == nil || !sandboxOpts.LiveMigrationSupportEnabled {
		return fmt.Errorf("sandbox is not configured to allow live migration: %w", errdefs.ErrFailedPrecondition)
	}

	if opts.MigrationOpts == nil {
		opts.MigrationOpts = &hcsschema.MigrationInitializeOptions{}
	}
	opts.MigrationOpts.Origin = opts.Origin

	// Prepare the running source VM; afterward it accepts only live-migration compatible calls.
	if err := opts.VMController.InitializeLiveMigrationOnSource(ctx, opts.MigrationOpts); err != nil {
		return fmt.Errorf("initialize live migration on source vm: %w", err)
	}

	c.sessionID = opts.SessionID
	c.origin = opts.Origin
	c.vmController = opts.VMController
	c.podControllers = opts.PodControllers
	c.state = StateSourcePrepared

	log.G(ctx).Info("migration source prepared")
	return nil
}

// ExportState captures the prepared source sandbox into an opaque, versioned
// saved state that the destination consumes via [Controller.ImportState]. The VM
// and per-pod payloads it carries are themselves opaque, owned by their
// respective controllers. A repeat call returns a fresh snapshot.
func (c *Controller) ExportState(ctx context.Context, sessionID string) (*anypb.Any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sessionID != sessionID {
		return nil, fmt.Errorf("session id %q does not match active session %q: %w", sessionID, c.sessionID, errdefs.ErrInvalidArgument)
	}

	// Allow re-export after a prior success so duplicate calls are idempotent.
	if c.state != StateSourcePrepared && c.state != StateSourceExported {
		return nil, fmt.Errorf("export requires state %s or %s (current: %s): %w", StateSourcePrepared, StateSourceExported, c.state, errdefs.ErrFailedPrecondition)
	}

	// Save the VM state.
	vmAny, err := c.vmController.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("save vm controller: %w", err)
	}

	// Save all the pod controller states as opaque envelopes.
	pods := make([]*anypb.Any, 0, len(c.podControllers))
	for podID, podCtrl := range c.podControllers {
		ps, err := podCtrl.Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("save pod %s: %w", podID, err)
		}

		pods = append(pods, ps)
	}

	// Wrap the VM and pod payloads in the versioned sandbox-level envelope.
	payload, err := proto.Marshal(&save.Payload{
		SchemaVersion: save.SchemaVersion,
		Vm:            vmAny,
		Pods:          pods,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal sandbox saved state: %w", err)
	}

	c.state = StateSourceExported

	log.G(ctx).Info("migration source state exported")
	return &anypb.Any{TypeUrl: save.TypeURL, Value: payload}, nil
}
