//go:build windows && lcow

package migration

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"
	"syscall"
	"time"

	"github.com/Microsoft/hcsshim/internal/controller/pod"
	"github.com/Microsoft/hcsshim/internal/controller/vm"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	hcs "github.com/Microsoft/hcsshim/internal/hcs/v2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/pkg/migration"
	"github.com/containerd/errdefs"
	"golang.org/x/sys/windows"
)

// defaultSocketReadyTimeout caps the Transfer wait for the duplicate socket.
const defaultSocketReadyTimeout = 10 * time.Minute

// Controller sequences a single live-migration session for an LCOW sandbox,
// driving the source or destination side through its lifecycle states.
type Controller struct {
	// mu guards all mutable fields below.
	mu sync.RWMutex

	// state is the session's current lifecycle state.
	state State

	// sessionID identifies the active migration session.
	sessionID string

	// sandboxID is the sandbox this session migrates; set on the destination.
	sandboxID string

	// origin is this host's role in the session: source or destination.
	origin hcsschema.MigrationOrigin

	// vmController drives the VM being migrated; borrowed from the service.
	vmController vmController

	// podControllers are the sandbox's pods keyed by pod ID; borrowed from the service.
	podControllers map[string]*pod.Controller

	// containerPodMapping aliases the service-owned containerID -> podID
	// index so ImportState and PatchResourcePaths can keep it in sync with
	// pod/container renames. Guarded by mu.
	containerPodMapping map[string]string

	// pendingPatches is the set of source container IDs imported by
	// ImportState that still need a PatchResourcePaths call; a container
	// drops out once it has been patched. Guarded by mu.
	pendingPatches map[string]struct{}

	// dupSocket is the duplicated transport socket used for the memory transfer.
	dupSocket windows.Handle

	// socketReady is closed once socket registration completes—on success or
	// failure—so a Transfer waiting on it unblocks and then proceeds or bails.
	socketReady chan struct{}

	// notifier is created lazily on the first Subscribe. Guarded by mu.
	notifier *notifications
}

// New returns an idle controller ready to host a migration session.
func New() *Controller {
	return &Controller{
		state:       StateIdle,
		socketReady: make(chan struct{}),
	}
}

// State returns the current session state.
func (c *Controller) State() State {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.state
}

// Transfer starts the session's memory transfer and returns immediately. The
// wait for the duplicate socket and the transfer itself run in the background;
// failures reach notification subscribers rather than the caller. Duplicate
// calls are no-ops.
func (c *Controller) Transfer(ctx context.Context, sessionID string, timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sessionID != sessionID {
		return fmt.Errorf("session id %q does not match active session %q: %w", sessionID, c.sessionID, errdefs.ErrInvalidArgument)
	}

	// Only one background goroutine should drive the transfer, so duplicate
	// Transfer calls must be no-ops. If the duplicate socket has not arrived
	// yet, the first call claims StateSocketWaiting and later calls return
	// below. If the socket has already arrived, the state is left unchanged here
	// and the goroutines dedup instead: only one goroutine moves
	// StateSocketReady to StateTransferring and the rest bail.
	switch c.state {
	case StateSocketReady:
		// Socket already arrived: fall through and start the transfer goroutine.
	case StateSourceExported, StateDestinationPrepared:
		// Socket not here yet: claim the wait so later calls no-op below.
		c.state = StateSocketWaiting
	case StateSocketWaiting, StateTransferring, StateTransferCompleted:
		// A transfer is already waiting, running, or done: nothing to do.
		return nil
	default:
		// No other state can begin a transfer.
		return fmt.Errorf("transfer not valid in state %s: %w", c.state, errdefs.ErrFailedPrecondition)
	}

	// Fall back to the default wait when the caller gives no timeout.
	if timeout <= 0 {
		timeout = defaultSocketReadyTimeout
	}
	socketReady := c.socketReady

	// Drive the transfer in the background so the caller returns immediately;
	// its outcome is surfaced to notification subscribers, not returned here.
	go func() {
		// Detached ctx so the transfer outlives the gRPC call.
		ctx = context.WithoutCancel(ctx)

		var err error
		// In case we encounter error from this point onwards, fail the migration.
		defer func() {
			if err != nil {
				c.failTransfer(ctx, err)
			}
		}()

		// Wait for the duplicate socket to arrive, or give up after the timeout.
		select {
		case <-socketReady:
		case <-time.After(timeout):
			// If the socket connection was not ready within timeout, return an error.
			err = fmt.Errorf("timed out waiting for socket ready after %s: %w", timeout, errdefs.ErrFailedPrecondition)
			return
		}

		// Take lock for checking and modifying the state.
		c.mu.Lock()

		// Gate: only a goroutine that still finds StateSocketReady claims the
		// transfer below (by moving to StateTransferring); any other state bails.
		if c.state != StateSocketReady {
			// StateFailed here means the session failed before the transfer could
			// start (e.g. destination socket registration failed after this goroutine
			// began waiting), so surface it as an error rather than returning silently.
			if c.state == StateFailed {
				err = fmt.Errorf("migration session failed before transfer could start: %w", errdefs.ErrFailedPrecondition)
			}

			c.mu.Unlock()
			return
		}

		// Move the state to transferring so that other concurrent callers
		// will return early from above and there is a single driver for transfer.
		c.state = StateTransferring

		// Unlock here prior to running the transfer so that other APIs can be called
		// while the long-running transfer is in progress.
		c.mu.Unlock()

		// Run the transfer; a failure marks the session failed for subscribers.
		if err = c.runTransfer(ctx); err != nil {
			return
		}

		c.mu.Lock()
		// A concurrent Cancel may have moved the session on while the lock was
		// released, so only a still-transferring session completes here.
		if c.state == StateTransferring {
			c.state = StateTransferCompleted
		}
		c.mu.Unlock()

		log.G(ctx).Info("migration transfer completed")
	}()

	return nil
}

// failTransfer marks the session failed and surfaces err to notification
// subscribers as a PHASE_FAILED event.
func (c *Controller) failTransfer(ctx context.Context, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Cancel takes precedence: a session canceled while the transfer ran owns
	// the outcome, so the expected transfer error is swallowed without failing.
	if c.state == StateCancelled {
		log.G(ctx).WithError(err).Debug("migration transfer error ignored; session cancelled")
		return
	}

	c.state = StateFailed

	log.G(ctx).WithError(err).Error("migration transfer failed")

	// If there are no subscribers, then we should return early.
	if c.notifier == nil {
		return
	}

	result := hcsschema.MigrationResultDestinationMigrationFailed
	if c.origin == hcsschema.MigrationOriginSource {
		result = hcsschema.MigrationResultSourceMigrationFailed
	}

	// Broadcast the failure to subscribers as a migration-failed event.
	c.notifier.broadcast(hcsschema.OperationSystemMigrationNotificationInfo{
		Origin: c.origin,
		Event:  hcsschema.MigrationEventMigrationFailed,
		Result: result,
	})
}

// sessionIDToUint32 derives a stable uint32 from a session ID. SHA-256 is
// deterministic and uniformly distributed, so the same session ID always maps
// to the same value on both hosts.
func sessionIDToUint32(sessionID string) uint32 {
	// Session IDs that parse as a GUID are normalized first so
	// formatting/case differences map to the same value.
	if g, err := guid.FromString(sessionID); err == nil {
		sessionID = g.String()
	}

	sum := sha256.Sum256([]byte(sessionID))
	return binary.BigEndian.Uint32(sum[:4])
}

// runTransfer issues the per-origin HCS calls for the memory transfer.
// Caller should ensure that it is called at most once.
func (c *Controller) runTransfer(ctx context.Context) error {
	config := &hcs.MigrationConfig{
		Socket:    syscall.Handle(c.dupSocket),
		SessionID: sessionIDToUint32(c.sessionID),
	}

	// Start the live migration on the VM according to this host's role.
	switch c.origin {
	case hcsschema.MigrationOriginSource:
		if err := c.vmController.StartLiveMigrationOnSource(ctx, config); err != nil {
			return fmt.Errorf("start live migration on source: %w", err)
		}
	case hcsschema.MigrationOriginDestination:
		if err := c.vmController.StartWithMigrationOptions(ctx, config); err != nil {
			return fmt.Errorf("start with migration options: %w", err)
		}
	default:
		return fmt.Errorf("unsupported migration origin %q: %w", c.origin, errdefs.ErrInvalidArgument)
	}

	// Begin streaming the memory transfer over the socket.
	transferOpts := &hcsschema.MigrationTransferOptions{Origin: c.origin}
	if err := c.vmController.StartLiveMigrationTransfer(ctx, transferOpts); err != nil {
		return fmt.Errorf("start live migration transfer: %w", err)
	}

	return nil
}

// Finalize applies the session's final outcome (resume or stop) on this host
// and, on success, leaves the session finalized so Cleanup can run. A repeat
// call for an already-finalized session is a no-op.
func (c *Controller) Finalize(ctx context.Context, sessionID string, action migration.FinalizeAction, events chan interface{}) error {
	if action == migration.FinalizeAction_FINALIZE_ACTION_UNSPECIFIED {
		return fmt.Errorf("finalize action must be specified: %w", errdefs.ErrInvalidArgument)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sessionID != sessionID {
		return fmt.Errorf("session id %q does not match active session %q: %w", sessionID, c.sessionID, errdefs.ErrInvalidArgument)
	}

	// Nothing to finalize, so return early: the session is already finalized
	// (idempotent repeat), idle (no active session), or a canceled destination,
	// which winds down directly through Cleanup rather than Finalize.
	if c.state == StateFinalized || c.state == StateIdle ||
		(c.origin == hcsschema.MigrationOriginDestination && c.state == StateCancelled) {
		return nil
	}

	// Finalize is valid only after a completed transfer, a cancellation or a failure.
	if c.state != StateTransferCompleted && c.state != StateCancelled && c.state != StateFailed {
		return fmt.Errorf("finalize not valid in state %s: %w", c.state, errdefs.ErrFailedPrecondition)
	}

	// Map the gRPC finalize action to the HCS final operation.
	var finalOp hcsschema.MigrationFinalOperation
	switch action {
	case migration.FinalizeAction_FINALIZE_ACTION_RESUME:
		finalOp = hcsschema.MigrationFinalOperationResume
	case migration.FinalizeAction_FINALIZE_ACTION_STOP:
		finalOp = hcsschema.MigrationFinalOperationStop
	default:
		return fmt.Errorf("unsupported finalize action %s: %w", action, errdefs.ErrInvalidArgument)
	}

	// Reconcile the requested operation against the VM's migration state. A first
	// finalize finds the VM still migrating and drives it; a retry after a
	// partially applied finalize finds the VM already settled and reconciles
	// against where it landed rather than re-issuing finalize.
	switch c.vmController.State() {
	case vm.StateTerminated:
		// A prior stop finalize already tore the VM down: a repeat stop is a
		// no-op, while a resume is no longer possible.
		if finalOp == hcsschema.MigrationFinalOperationResume {
			return fmt.Errorf("cannot resume finalize: VM already terminated by a stop finalize: %w", errdefs.ErrFailedPrecondition)
		}
	case vm.StateMigrationFinalized:
		// A prior resume finalize already completed on the VM: a stop now tears
		// it down, while a repeat resume falls through to re-run the resume steps.
		if finalOp == hcsschema.MigrationFinalOperationStop {
			if err := c.vmController.TerminateVM(ctx); err != nil {
				return fmt.Errorf("terminate resumed vm on stop finalize: %w", err)
			}
		}
	default:
		// The VM is still migrating: apply the final operation.
		if err := c.vmController.FinalizeLiveMigration(ctx,
			&hcsschema.MigrationFinalizedOptions{
				Origin:             c.origin,
				FinalizedOperation: finalOp,
			}); err != nil {
			return fmt.Errorf("finalize live migration (origin=%s, action=%s): %w", c.origin, action, err)
		}
	}

	// On RESUME, resume the VM (the source also rebuilds its GCS bridge), then
	// resume every pod on both sides to reattach containers and lift the freeze.
	if finalOp == hcsschema.MigrationFinalOperationResume {
		switch c.origin {
		case hcsschema.MigrationOriginDestination:
			// Resume the destination VM.
			if err := c.vmController.Resume(ctx, false /* rebuildBridge */); err != nil {
				return fmt.Errorf("resume destination vm from migration: %w", err)
			}

			// Reattach each migrated pod's containers and processes on this host.
			for podID, podCtrl := range c.podControllers {
				if err := podCtrl.Resume(ctx, c.vmController, events, true /* isDestination */); err != nil {
					return fmt.Errorf("resume pod %q from migration: %w", podID, err)
				}
			}
		case hcsschema.MigrationOriginSource:
			// Resume the source VM, rebuilding its GCS bridge.
			if err := c.vmController.Resume(ctx, true /* rebuildBridge */); err != nil {
				return fmt.Errorf("resume source vm from migration: %w", err)
			}

			// Resume pods to lift the source freeze, skipping destination-only
			// steps such as network reset.
			for podID, podCtrl := range c.podControllers {
				if err := podCtrl.Resume(ctx, c.vmController, events, false /* isDestination */); err != nil {
					return fmt.Errorf("resume pod %q from migration: %w", podID, err)
				}
			}
		}
	}

	// Mark the session finalized so Cleanup can run and repeat calls no-op.
	c.state = StateFinalized

	log.G(ctx).WithField(logfields.Action, action.String()).Info("migration session finalized")
	return nil
}

// Cancel aborts an in-flight migration session, marking it cancelled so a
// subsequent Finalize and Cleanup can wind it down. A repeat call is a no-op.
func (c *Controller) Cancel(ctx context.Context, sessionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sessionID != sessionID {
		return fmt.Errorf("session id %q does not match active session %q: %w", sessionID, c.sessionID, errdefs.ErrInvalidArgument)
	}

	// Idempotent: already canceled or not initialized.
	if c.state == StateIdle || c.state == StateCancelled || c.state == StateCancelling {
		return nil
	}

	// The destination imported the snapshot but never materialized the HCS
	// compute system (that happens in PrepareDestination), so there is no live
	// migration to abort. Mark it canceled directly so Cleanup can wind the
	// session down.
	if c.state == StateDestinationImported {
		c.state = StateCancelled
		log.G(ctx).Info("migration session cancelled")
		return nil
	}

	// Enter the cancelling state before the abort so a concurrent transfer yields
	// instead of completing; a successful abort then marks the session cancelled.
	c.state = StateCancelling

	if err := c.cancelLiveMigration(ctx); err != nil {
		// Abort did not take hold; mark failed so the caller can retry Cancel.
		c.state = StateFailed
		return fmt.Errorf("cancel live migration: %w", err)
	}

	c.state = StateCancelled

	log.G(ctx).Info("migration session cancelled")
	return nil
}

// cancelLiveMigration issues the HCS abort with c.mu released so the possibly
// blocking call does not stall other callers, re-acquiring it before returning.
// Must be called with c.mu held.
func (c *Controller) cancelLiveMigration(ctx context.Context) error {
	c.mu.Unlock()
	defer c.mu.Lock()
	return c.vmController.CancelLiveMigration(ctx)
}

// Cleanup is the terminal call of a migration session on either side. It
// reverts the controller back to [StateIdle], releasing the transport socket
// and tearing down notification subscribers, regardless of whether the
// migration completed, failed, or was cancelled. A call against an already-idle
// controller is a no-op.
func (c *Controller) Cleanup(ctx context.Context, sessionID string, events chan interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Already cleaned up / no active session.
	if c.state == StateIdle {
		return nil
	}

	if c.sessionID != sessionID {
		return fmt.Errorf("session id %q does not match active session %q: %w", sessionID, c.sessionID, errdefs.ErrInvalidArgument)
	}

	// Cleanup is valid only from a settled state:
	//   - StateFinalized: either side after a successful Finalize. The normal
	//     path finalizes both ends; a cancel or error still finalizes the source.
	//   - StateCancelled: the destination only, whose Finalize is a no-op, so it
	//     cleans up directly from the canceled state.
	canCleanup := c.state == StateFinalized ||
		(c.origin == hcsschema.MigrationOriginDestination && c.state == StateCancelled)
	if !canCleanup {
		return fmt.Errorf("cleanup not valid for %s in state %s: %w", c.origin, c.state, errdefs.ErrFailedPrecondition)
	}

	// The transport socket is unused past this point and the notifier is
	// scoped to the session, so release both before resetting state.
	if c.dupSocket != 0 {
		if err := windows.Closesocket(c.dupSocket); err != nil {
			log.G(ctx).WithError(err).Warn("close duplicate migration socket")
		}
		c.dupSocket = 0
	}

	// Tear down the notifier: stop the forwarder and close all subscriber streams.
	if c.notifier != nil {
		c.notifier.close()
		c.notifier = nil
	}

	// Reset all session-scoped state so the controller can host a new session.
	c.sessionID, c.sandboxID, c.origin = "", "", ""
	c.vmController = nil
	c.podControllers = nil
	c.containerPodMapping = nil
	c.pendingPatches = nil
	c.socketReady = make(chan struct{})
	c.state = StateIdle

	log.G(ctx).Info("migration session cleaned up")
	return nil
}
