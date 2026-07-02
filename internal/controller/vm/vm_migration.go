//go:build windows && (lcow || wcow)

package vm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Microsoft/go-winio"

	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	hcs "github.com/Microsoft/hcsshim/internal/hcs/v2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
)

// compatibilityInfoProperty is the HCS property name used to retrieve the
// VM's opaque migration-compatibility blob via PropertiesV3.
const compatibilityInfoProperty = "CompatibilityInfo"

// InitializeLiveMigrationOnSource prepares the running source VM for an
// outgoing live migration. Once it succeeds the VM accepts only live-migration
// calls until the migration completes or is rolled back.
func (c *Controller) InitializeLiveMigrationOnSource(ctx context.Context, options *hcsschema.MigrationInitializeOptions) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Only a running VM can begin a migration.
	if c.vmState != StateRunning {
		return fmt.Errorf("cannot initialize live migration on source: VM is in state %s", c.vmState)
	}

	// Hand the initialize request to the HCS for the UVM.
	if err := c.uvm.InitializeLiveMigrationOnSource(ctx, options); err != nil {
		return fmt.Errorf("failed to initialize live migration on source: %w", err)
	}

	// From here on only live-migration APIs are permitted.
	c.vmState = StateSourceMigrating
	log.G(ctx).WithField(logfields.UVMID, c.vmID).Debug("initialized live migration on source")

	return nil
}

// CompatibilityInfo returns the opaque, source-emitted blob that the destination
// hands back to HCS when starting the target VM, letting the platform confirm
// the two hosts can interchange live-migration state. Available while the VM is
// running or migrating.
func (c *Controller) CompatibilityInfo(ctx context.Context) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// The blob is read from a live source VM, including once it has begun migrating.
	if c.vmState != StateRunning && c.vmState != StateSourceMigrating {
		return nil, fmt.Errorf("cannot query compatibility info: VM is in state %s", c.vmState)
	}

	// Ask the HCS for the compatibility property.
	props, err := c.uvm.PropertiesV3(ctx, &hcsschema.PropertyQuery{
		Queries: map[string]interface{}{compatibilityInfoProperty: nil},
	})
	if err != nil {
		return nil, fmt.Errorf("query compatibility info: %w", err)
	}

	// Pull the raw blob out of the property response.
	resp, ok := props.PropertyResponses[compatibilityInfoProperty]
	if !ok || len(resp.Response) == 0 {
		return nil, fmt.Errorf("compatibility info not present in property response")
	}

	// Decode the opaque payload and return its bytes to the caller.
	var info hcsschema.CompatibilityInfo
	if err := json.Unmarshal(resp.Response, &info); err != nil {
		return nil, fmt.Errorf("decode compatibility info: %w", err)
	}

	log.G(ctx).WithField(logfields.UVMID, c.vmID).Debugf("queried compatibility info")
	return info.Data, nil
}

// MigrationNotifications returns the VM's live-migration event channel. The
// channel lives for the VM's lifetime, so callers can subscribe any time after
// the VM is created and will not miss early events.
func (c *Controller) MigrationNotifications() (<-chan hcsschema.OperationSystemMigrationNotificationInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Notifications are valid from creation through the migration window,
	// on both the source and destination sides.
	if c.vmState != StateCreated && c.vmState != StateRunning &&
		c.vmState != StateSourceMigrating && c.vmState != StateMigratingCreated &&
		c.vmState != StateDestinationMigrating {
		return nil, fmt.Errorf("cannot query migration notifications: VM is in state %s", c.vmState)
	}

	return c.uvm.MigrationNotifications(), nil
}

// StartWithMigrationOptions starts the VM as the destination of a live
// migration over the supplied transport socket. On return the VM is migrating
// and awaiting the source's state transfer.
func (c *Controller) StartWithMigrationOptions(ctx context.Context, config *hcs.MigrationConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Destination start is only valid on a created migrating VM.
	if c.vmState != StateMigratingCreated {
		return fmt.Errorf("cannot start with migration options: VM is in state %s", c.vmState)
	}

	// Arm the host-side GCS listener before start so the guest's dial cannot race it.
	if err := c.guest.PrepareConnection(winio.VsockServiceID(prot.LinuxGcsVsockPort)); err != nil {
		return fmt.Errorf("prepare destination gcs connection: %w", err)
	}

	// Start the destination VM against the migration socket.
	if err := c.uvm.StartWithMigrationOptions(ctx, config); err != nil {
		return fmt.Errorf("failed to start with migration options: %w", err)
	}

	// Watch for VM exit in the background.
	go c.waitForVMExit(ctx)
	c.vmState = StateDestinationMigrating

	log.G(ctx).WithField(logfields.UVMID, c.vmID).Debug("started destination VM with migration options")
	return nil
}

// StartLiveMigrationOnSource begins the source side of the migration over the
// supplied transport socket. The memory-transfer phase is driven separately via
// [Controller.StartLiveMigrationTransfer].
func (c *Controller) StartLiveMigrationOnSource(ctx context.Context, config *hcs.MigrationConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Source start is only valid once the migration has been initialized.
	if c.vmState != StateSourceMigrating {
		return fmt.Errorf("cannot start live migration on source: VM is in state %s", c.vmState)
	}

	// Tolerate the blackout-induced hvsock drop; cleared in [FinalizeLiveMigration].
	c.guest.SetMigrating(true)
	if err := c.uvm.StartLiveMigrationOnSource(ctx, config); err != nil {
		// Roll the flag back if the host rejects the start.
		c.guest.SetMigrating(false)
		return fmt.Errorf("failed to start live migration on source: %w", err)
	}

	log.G(ctx).WithField(logfields.UVMID, c.vmID).Debug("started live migration on source")
	return nil
}

// StartLiveMigrationTransfer drives the memory-transfer phase of an in-progress
// migration. Progress is reported through [Controller.MigrationNotifications].
func (c *Controller) StartLiveMigrationTransfer(ctx context.Context, options *hcsschema.MigrationTransferOptions) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Transfer is only valid mid-migration, on either source or destination.
	if c.vmState != StateSourceMigrating && c.vmState != StateDestinationMigrating {
		return fmt.Errorf("cannot start live migration transfer: VM is in state %s", c.vmState)
	}

	if err := c.uvm.StartLiveMigrationTransfer(ctx, options); err != nil {
		return fmt.Errorf("failed to start live migration transfer: %w", err)
	}

	log.G(ctx).WithField(logfields.UVMID, c.vmID).Debug("started live migration memory transfer")
	return nil
}

// FinalizeLiveMigration completes the migration. A Stop finalize tears down the
// stopped side (the source in the forward flow, the destination in the reverse);
// a Resume finalize returns control to the caller, who must then call [Controller.Resume].
func (c *Controller) FinalizeLiveMigration(ctx context.Context, options *hcsschema.MigrationFinalizedOptions) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Finalize is only valid mid-migration, on either source or destination.
	if c.vmState != StateSourceMigrating && c.vmState != StateDestinationMigrating {
		return fmt.Errorf("cannot finalize live migration: VM is in state %s", c.vmState)
	}

	if err := c.uvm.FinalizeLiveMigration(ctx, options); err != nil {
		return fmt.Errorf("failed to finalize live migration: %w", err)
	}

	// On a finalize Stop, drain the stopped side's VM (source in the forward
	// flow, destination in the reverse) to termination.
	if options != nil && options.FinalizedOperation == hcsschema.MigrationFinalOperationStop {
		// Source stop: lift the Save-time freeze so the defunct containers'
		// scratch-layer unmap on delete is no longer rejected.
		if options.Origin == hcsschema.MigrationOriginSource && c.scsiController != nil {
			c.scsiController.Resume(ctx, c.uvm, c.guest)
		}

		c.guest.SetMigrating(false)
		_ = c.uvm.Wait(ctx)
		c.vmState = StateTerminated

		log.G(ctx).WithField(logfields.UVMID, c.vmID).Debug("finalized live migration: VM terminated")
		return nil
	}

	log.G(ctx).WithField(logfields.UVMID, c.vmID).Debug("finalized live migration")
	return nil
}
