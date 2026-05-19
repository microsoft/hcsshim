//go:build windows

package hcs

import (
	"context"
	"encoding/json"
	"errors"
	"syscall"
	"time"

	"github.com/Microsoft/hcsshim/internal/computecore"
	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/oc"

	"go.opencensus.io/trace"
)

// MigrationConfig holds parameters for starting a compute system as a live migration
// destination, or for initiating the source side of a live migration.
type MigrationConfig struct {
	// Socket is the handle to the live migration transport socket.
	Socket syscall.Handle
	// SessionID identifies the migration session.
	SessionID uint32
}

// StartWithMigrationOptions synchronously starts the compute system as a live
// migration destination using the provided configuration.
func (computeSystem *System) StartWithMigrationOptions(ctx context.Context, config *MigrationConfig) (err error) {
	if config == nil {
		return errors.New("live migration config must not be nil")
	}

	operation := "hcs::System::Start"

	ctx, span := oc.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.Lock()
	defer computeSystem.handleLock.Unlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
	}

	opts, err := json.Marshal(hcsschema.StartOptions{
		DestinationMigrationOptions: &hcsschema.MigrationStartOptions{
			NetworkSettings: &hcsschema.MigrationNetworkSettings{SessionID: config.SessionID},
		},
	})
	if err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}

	resultJSON, callErr := runOperation(ctx, func(op computecore.HcsOperation) error {
		if err := computecore.HcsAddResourceToOperation(ctx, op, computecore.HcsResourceTypeSocket, resourcepaths.LiveMigrationSocketURI, config.Socket); err != nil {
			return err
		}
		return computecore.HcsStartComputeSystem(ctx, computeSystem.handle, op, string(opts))
	})
	if callErr != nil {
		return makeSystemError(computeSystem, operation, callErr, processHcsResult(ctx, resultJSON))
	}
	computeSystem.startTime = time.Now()
	return nil
}

// InitializeLiveMigrationOnSource prepares the source compute system for a
// live migration. Must be called on the source before StartLiveMigrationOnSource.
func (computeSystem *System) InitializeLiveMigrationOnSource(ctx context.Context, options *hcsschema.MigrationInitializeOptions) (err error) {
	operation := "hcs::System::InitializeLiveMigrationOnSource"

	ctx, span := oc.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.Lock()
	defer computeSystem.handleLock.Unlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
	}

	if options == nil {
		options = &hcsschema.MigrationInitializeOptions{}
	}
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}

	op, err := computecore.HcsCreateOperation(ctx, 0, 0)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	defer computecore.HcsCloseOperation(ctx, op)

	// Issue the initialize call and wait for completion.
	if err = computecore.HcsInitializeLiveMigrationOnSource(ctx, computeSystem.handle, op, string(optionsJSON)); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	if _, err = computecore.HcsWaitForOperationResult(ctx, op, 0xFFFFFFFF); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	return nil
}

// StartLiveMigrationOnSource begins the source-side migration using the given
// transport socket and session ID. Blocks until HCS accepts the start;
// transfer progress is observed via MigrationNotifications.
func (computeSystem *System) StartLiveMigrationOnSource(ctx context.Context, config *MigrationConfig) (err error) {
	if config == nil {
		return errors.New("migration config must not be nil")
	}

	operation := "hcs::System::StartLiveMigrationOnSource"

	ctx, span := oc.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.Lock()
	defer computeSystem.handleLock.Unlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
	}

	op, err := computecore.HcsCreateOperation(ctx, 0, 0)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	defer computecore.HcsCloseOperation(ctx, op)

	// Attach the migration socket to the operation before starting.
	if err := computecore.HcsAddResourceToOperation(ctx, op, computecore.HcsResourceTypeSocket, resourcepaths.LiveMigrationSocketURI, config.Socket); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}

	options := hcsschema.MigrationStartOptions{
		NetworkSettings: &hcsschema.MigrationNetworkSettings{SessionID: config.SessionID},
	}
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}

	// Issue the start call and wait for completion.
	if err := computecore.HcsStartLiveMigrationOnSource(ctx, computeSystem.handle, op, string(optionsJSON)); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	if _, err := computecore.HcsWaitForOperationResult(ctx, op, 0xFFFFFFFF); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	return nil
}

// StartLiveMigrationTransfer starts the memory transfer phase of a live migration.
func (computeSystem *System) StartLiveMigrationTransfer(ctx context.Context, options *hcsschema.MigrationTransferOptions) (err error) {
	operation := "hcs::System::StartLiveMigrationTransfer"

	ctx, span := oc.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.Lock()
	defer computeSystem.handleLock.Unlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
	}

	if options == nil {
		options = &hcsschema.MigrationTransferOptions{}
	}
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}

	op, err := computecore.HcsCreateOperation(ctx, 0, 0)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	defer computecore.HcsCloseOperation(ctx, op)

	// Begin the memory transfer and wait for completion.
	if err := computecore.HcsStartLiveMigrationTransfer(ctx, computeSystem.handle, op, string(optionsJSON)); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	if _, err := computecore.HcsWaitForOperationResult(ctx, op, 0xFFFFFFFF); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	return nil
}

// FinalizeLiveMigration completes the live migration workflow. If resume is true the VM
// is resumed on the destination; otherwise it is stopped.
func (computeSystem *System) FinalizeLiveMigration(ctx context.Context, resume bool) (err error) {
	operation := "hcs::System::FinalizeLiveMigration"

	ctx, span := oc.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.Lock()
	defer computeSystem.handleLock.Unlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
	}

	// Choose whether to resume or stop the VM after migration.
	finalOp := hcsschema.MigrationFinalOperationStop
	if resume {
		finalOp = hcsschema.MigrationFinalOperationResume
	}
	optionsJSON, err := json.Marshal(hcsschema.MigrationFinalizedOptions{FinalizedOperation: finalOp})
	if err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}

	op, err := computecore.HcsCreateOperation(ctx, 0, 0)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	defer computecore.HcsCloseOperation(ctx, op)

	// Finalize the migration and wait for completion.
	if err := computecore.HcsFinalizeLiveMigration(ctx, computeSystem.handle, op, string(optionsJSON)); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	if _, err := computecore.HcsWaitForOperationResult(ctx, op, 0xFFFFFFFF); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	return nil
}

// MigrationNotifications returns a read-only channel of live migration events
// for this System. The channel exists for the System's lifetime and is safe
// to subscribe to before any migration call, so callers do not miss early
// events such as SetupDone.
//
// The channel is never closed; callers signal end-of-stream via their own
// context. Sends are non-blocking (buffer size migrationNotificationBufferSize)
// and events are dropped on overflow, so consumers must drain promptly.
func (computeSystem *System) MigrationNotifications() <-chan hcsschema.OperationSystemMigrationNotificationInfo {
	return computeSystem.migrationNotifyCh
}
