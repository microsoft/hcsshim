//go:build windows

package hcs

import (
	"context"
	"encoding/json"
	"errors"
	"syscall"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/computecore"
	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/oc"

	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
	"golang.org/x/sys/windows"
)

// migrationNotificationBufferSize is the capacity of the LM notification channel.
const migrationNotificationBufferSize = 16

// MigrationConfig holds parameters for starting a compute system as a live migration
// destination, or for initiating the source side of a live migration.
type MigrationConfig struct {
	// Socket is the handle to the live migration transport socket.
	Socket syscall.Handle
	// SessionID identifies the migration session.
	SessionID uint32
}

// migrationCallback is the syscall callback registered with HcsSetComputeSystemCallback
// for live migration events. It receives events and dispatches them to the channel
// stored in the System via the callbackContext pointer.
var migrationCallback = syscall.NewCallback(migrationCallbackHandler)

// migrationCallbackHandler is invoked by computecore.dll for live migration events.
// ctx is &computeSystem.migrationNotifyCh, kept alive across the cgo boundary by
// computeSystem.migrationPinner (unpinned only after HcsCloseComputeSystem has
// drained any in-flight callbacks). The notification channel is never closed.
// Skipping the close keeps tear-down trivially safe and removes the only
// thing that could turn a channel send into a panic.
func migrationCallbackHandler(eventPtr uintptr, ctx uintptr) uintptr {
	if eventPtr == 0 || ctx == 0 {
		return 0
	}

	e := (*computecore.HcsEvent)(unsafe.Pointer(eventPtr))
	ch := *(*chan string)(unsafe.Pointer(ctx))

	eventData := ""
	if e.EventData != nil {
		eventData = windows.UTF16PtrToString(e.EventData)
	}

	logrus.WithFields(logrus.Fields{
		"event-type": e.Type.String(),
		"event-data": eventData,
	}).Debug("HCS migration notification")

	// Non-blocking send to avoid blocking the HCS callback thread.
	select {
	case ch <- eventData:
	default:
		logrus.WithField("event-type", e.Type.String()).Warn("migration notification channel full, dropping event")
	}

	return 0
}

// openMigrationHandle opens a computecore handle to the same system and
// registers a callback for live migration events. It populates
// computeSystem.migrationHandle and computeSystem.migrationNotifyCh.
//
// The caller MUST hold computeSystem.handleLock.
func (computeSystem *System) openMigrationHandle(ctx context.Context) error {
	if computeSystem.migrationHandle != 0 {
		// Already open — idempotent.
		return nil
	}

	// Sanity check: the primary handle must be valid.
	if computeSystem.handle == 0 {
		return ErrAlreadyClosed
	}

	// Open a second handle via computecore for LM operations and events.
	handle, err := computecore.HcsOpenComputeSystem(ctx, computeSystem.id, syscall.GENERIC_ALL)
	if err != nil {
		return err
	}

	// Create the notification channel and store it on the struct.
	computeSystem.migrationHandle = handle
	computeSystem.migrationNotifyCh = make(chan string, migrationNotificationBufferSize)

	// Pin the address of the notification channel field so it stays visible
	// to the GC while HCS holds it as a uintptr callback context. Without
	// pinning, this would violate cgo's pointer-passing rules.
	computeSystem.migrationPinner.Pin(&computeSystem.migrationNotifyCh)

	// Register the callback.
	if err := computecore.HcsSetComputeSystemCallback(ctx, handle, computecore.HcsEventOptionEnableLiveMigrationEvents, uintptr(unsafe.Pointer(&computeSystem.migrationNotifyCh)), migrationCallback); err != nil {
		computeSystem.migrationPinner.Unpin()
		computeSystem.migrationNotifyCh = nil
		computeSystem.migrationHandle = 0
		computecore.HcsCloseComputeSystem(ctx, handle)
		return err
	}
	return nil
}

// closeMigrationHandle unregisters the LM callback and closes the migration
// handle.
//
// The caller MUST hold computeSystem.handleLock.
func (computeSystem *System) closeMigrationHandle(ctx context.Context) {
	if computeSystem.migrationHandle == 0 {
		return
	}

	// Unregister callback by passing zeros, then close the compute system.
	// HcsCloseComputeSystem waits for any in-flight callbacks to return, so
	// after it completes no callback can still be reading the pinned
	// channel pointer and it is safe to Unpin.
	_ = computecore.HcsSetComputeSystemCallback(ctx, computeSystem.migrationHandle, computecore.HcsEventOptionNone, 0, 0)
	computecore.HcsCloseComputeSystem(ctx, computeSystem.migrationHandle)
	computeSystem.migrationHandle = 0

	computeSystem.migrationPinner.Unpin()

	// Drop the channel reference. The channel is intentionally not closed:
	// consumers signal end-of-stream via the System's context, so a close
	// would add no information and would only complicate tear-down.
	computeSystem.migrationNotifyCh = nil
}

// StartWithMigrationOptions synchronously starts the compute system as a live
// migration destination using the provided configuration.
func (computeSystem *System) StartWithMigrationOptions(ctx context.Context, config *MigrationConfig) (err error) {
	if config == nil {
		return errors.New("live migration config must not be nil")
	}

	operation := "hcs::System::Start"

	computeSystem.handleLock.Lock()
	defer computeSystem.handleLock.Unlock()

	if computeSystem.handle == 0 {
		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
	}

	// Open the migration handle for LM events and operations.
	if err := computeSystem.openMigrationHandle(ctx); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	defer func() {
		if err != nil {
			computeSystem.closeMigrationHandle(ctx)
		}
	}()

	// Create a computecore operation to track the start request.
	op, err := computecore.HcsCreateOperation(ctx, 0, 0)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	defer computecore.HcsCloseOperation(ctx, op)

	// Attach the live migration socket to the operation.
	if err := computecore.HcsAddResourceToOperation(ctx, op, computecore.HcsResourceTypeSocket, resourcepaths.LiveMigrationSocketURI, config.Socket); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}

	// Build start options with destination migration settings.
	options := hcsschema.StartOptions{
		DestinationMigrationOptions: &hcsschema.MigrationStartOptions{
			NetworkSettings: &hcsschema.MigrationNetworkSettings{SessionID: config.SessionID},
		},
	}
	raw, err := json.Marshal(options)
	if err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}

	return computeSystem.start(ctx, op, string(raw))
}

// InitializeLiveMigrationOnSource initializes a live migration on the source side with the given options.
func (computeSystem *System) InitializeLiveMigrationOnSource(ctx context.Context, options *hcsschema.MigrationInitializeOptions) (err error) {
	operation := "hcs::System::InitializeLiveMigrationOnSource"

	ctx, span := oc.StartSpan(ctx, operation)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", computeSystem.id))

	computeSystem.handleLock.Lock()
	defer computeSystem.handleLock.Unlock()

	// Open the migration handle for LM events and operations.
	if err = computeSystem.openMigrationHandle(ctx); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	defer func() {
		if err != nil {
			computeSystem.closeMigrationHandle(ctx)
		}
	}()

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
	if err = computecore.HcsInitializeLiveMigrationOnSource(ctx, computeSystem.migrationHandle, op, string(optionsJSON)); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	if _, err = computecore.HcsWaitForOperationResult(ctx, op, 0xFFFFFFFF); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	return nil
}

// StartLiveMigrationOnSource starts the live migration on the source side using the provided
// transport socket and session ID.
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

	if computeSystem.migrationHandle == 0 {
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
	if err := computecore.HcsStartLiveMigrationOnSource(ctx, computeSystem.migrationHandle, op, string(optionsJSON)); err != nil {
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

	if computeSystem.migrationHandle == 0 {
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
	if err := computecore.HcsStartLiveMigrationTransfer(ctx, computeSystem.migrationHandle, op, string(optionsJSON)); err != nil {
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

	if computeSystem.migrationHandle == 0 {
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
	if err := computecore.HcsFinalizeLiveMigration(ctx, computeSystem.migrationHandle, op, string(optionsJSON)); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}
	if _, err := computecore.HcsWaitForOperationResult(ctx, op, 0xFFFFFFFF); err != nil {
		return makeSystemError(computeSystem, operation, err, nil)
	}

	// Migration is complete — release the migration handle and callback.
	computeSystem.closeMigrationHandle(ctx)
	return nil
}

// MigrationNotifications returns a read-only channel that receives live migration
// event data strings. Returns an error if no migration handle is open.
func (computeSystem *System) MigrationNotifications() (<-chan string, error) {
	computeSystem.handleLock.RLock()
	defer computeSystem.handleLock.RUnlock()

	if computeSystem.migrationHandle == 0 {
		return nil, errors.New("migration handle not open; call StartWithMigrationOptions or InitializeLiveMigrationOnSource first")
	}
	return computeSystem.migrationNotifyCh, nil
}
