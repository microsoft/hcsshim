//go:build windows && (lcow || wcow)

package vmmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// StartWithMigrationOptions starts the utility VM as a live migration destination
// using the provided migration configuration.
func (uvm *UtilityVM) StartWithMigrationOptions(ctx context.Context, config *hcs.MigrationConfig) error {
	if err := uvm.cs.StartWithMigrationOptions(ctx, config); err != nil {
		return fmt.Errorf("failed to start utility VM with migration options: %w", err)
	}
	return nil
}

// InitializeLiveMigrationOnSource initializes a live migration on the source side
// of the utility VM with the provided options.
func (uvm *UtilityVM) InitializeLiveMigrationOnSource(ctx context.Context, options *hcsschema.MigrationInitializeOptions) error {
	if err := uvm.cs.InitializeLiveMigrationOnSource(ctx, options); err != nil {
		return fmt.Errorf("failed to initialize live migration on source: %w", err)
	}
	return nil
}

// StartLiveMigrationOnSource starts the live migration on the source side using
// the provided transport socket and session ID.
func (uvm *UtilityVM) StartLiveMigrationOnSource(ctx context.Context, config *hcs.MigrationConfig) error {
	if err := uvm.cs.StartLiveMigrationOnSource(ctx, config); err != nil {
		return fmt.Errorf("failed to start live migration on source: %w", err)
	}
	return nil
}

// StartLiveMigrationTransfer starts the memory transfer phase of a live migration.
func (uvm *UtilityVM) StartLiveMigrationTransfer(ctx context.Context, options *hcsschema.MigrationTransferOptions) error {
	if err := uvm.cs.StartLiveMigrationTransfer(ctx, options); err != nil {
		return fmt.Errorf("failed to start live migration transfer: %w", err)
	}
	return nil
}

// FinalizeLiveMigration completes the live migration workflow. If resume is true
// the utility VM is resumed; otherwise it is stopped.
func (uvm *UtilityVM) FinalizeLiveMigration(ctx context.Context, resume bool) error {
	if err := uvm.cs.FinalizeLiveMigration(ctx, resume); err != nil {
		return fmt.Errorf("failed to finalize live migration: %w", err)
	}
	return nil
}

// MigrationNotifications returns a read-only channel that receives live migration
// event payloads for the utility VM.
func (uvm *UtilityVM) MigrationNotifications() (<-chan hcsschema.OperationSystemMigrationNotificationInfo, error) {
	ch, err := uvm.cs.MigrationNotifications()
	if err != nil {
		return nil, fmt.Errorf("failed to get migration notifications channel: %w", err)
	}
	return ch, nil
}
