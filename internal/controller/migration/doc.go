//go:build windows && lcow

// Package migration provides a controller for sequencing a single live-migration
// session of an LCOW sandbox between two shims: a source that hands off a running
// sandbox and a destination that receives it.
//
// The [Controller] drives one session at a time. The source captures its sandbox
// as an opaque snapshot ([Controller.PrepareSource], [Controller.ExportState]);
// the destination rehydrates that snapshot ([Controller.ImportState]), rebinds
// each migrated container onto its own IDs ([Controller.PatchResourcePaths]), and
// materializes the VM ([Controller.PrepareDestination]). Once both sides share the
// duplicated transport socket ([Controller.RegisterDuplicateSocket]), the memory
// transfer runs ([Controller.Transfer]); the session is then committed with a
// resume or stop ([Controller.Finalize]) and torn down ([Controller.Cleanup]).
// The VM and pod controllers it drives are owned by the service and only borrowed
// for the session.
//
// # Lifecycle
//
// A session created via [New] starts at [StateIdle] and follows a source or a
// destination path; the two converge once the transport socket is ready:
//
//	        source                                destination
//	┌─────────────────────┐              ┌───────────────────────────┐
//	│      StateIdle      │              │         StateIdle         │
//	└──────────┬──────────┘              └─────────────┬─────────────┘
//	           │ PrepareSource                         │ ImportState
//	           ▼                                       ▼
//	┌─────────────────────┐              ┌───────────────────────────┐
//	│ StateSourcePrepared │              │ StateDestinationImported  │
//	└──────────┬──────────┘              └─────────────┬─────────────┘
//	           │ ExportState                           │ PatchResourcePaths (per container)
//	           ▼                                       │ then PrepareDestination
//	┌─────────────────────┐                            ▼
//	│ StateSourceExported │              ┌───────────────────────────┐
//	└──────────┬──────────┘              │ StateDestinationPrepared  │
//	           │                         └─────────────┬─────────────┘
//	           │ RegisterDuplicateSocket               │ RegisterDuplicateSocket
//	           └───────────────┬───────────────────────┘
//	                           ▼
//	                ┌────────────────────┐
//	                │  StateSocketReady  │
//	                └─────────┬──────────┘
//	                          │ Transfer
//	                          ▼
//	                ┌────────────────────┐
//	                │ StateTransferring  │
//	                └─────────┬──────────┘
//	                          │ transfer ok
//	                          ▼
//	              ┌────────────────────────┐
//	              │ StateTransferCompleted │
//	              └───────────┬────────────┘
//	                          │ Finalize
//	                          ▼
//	                 ┌──────────────────┐
//	                 │  StateFinalized  │
//	                 └────────┬─────────┘
//	                          │ Cleanup
//	                          ▼
//	                   ┌──────────────┐
//	                   │  StateIdle   │
//	                   └──────────────┘
//
// If [Controller.Transfer] is called before the socket is registered, the session
// waits in [StateSocketWaiting] and advances to [StateTransferring] once
// [Controller.RegisterDuplicateSocket] supplies the socket.
//
// State descriptions:
//
//   - [StateIdle]: no session is active; the initial state and where
//     [Controller.Cleanup] returns the controller.
//   - [StateSourcePrepared]: [Controller.PrepareSource] has armed the source; the
//     next call is [Controller.ExportState].
//   - [StateSourceExported]: [Controller.ExportState] has produced the opaque
//     sandbox snapshot; the next calls are [Controller.RegisterDuplicateSocket]
//     and [Controller.Transfer], in either order.
//   - [StateDestinationImported]: [Controller.ImportState] has rehydrated the
//     snapshot; the next calls are [Controller.PatchResourcePaths] (one per
//     container) followed by [Controller.PrepareDestination].
//   - [StateDestinationPrepared]: [Controller.PrepareDestination] has materialized
//     the destination HCS compute system; the next calls are
//     [Controller.RegisterDuplicateSocket] and [Controller.Transfer], in either order.
//   - [StateSocketReady]: the duplicated transport socket has been adopted via
//     [Controller.RegisterDuplicateSocket]; the next call is [Controller.Transfer].
//   - [StateSocketWaiting]: [Controller.Transfer] ran before the socket arrived and
//     is waiting for it in the background; it advances to [StateTransferring] once
//     the socket is registered.
//   - [StateTransferring]: [Controller.Transfer] has started the memory transfer;
//     it auto-advances to [StateTransferCompleted] or [StateFailed].
//   - [StateTransferCompleted]: the memory transfer finished; the next call is
//     [Controller.Finalize].
//   - [StateFinalized]: [Controller.Finalize] has applied the resume or stop; the
//     next call is [Controller.Cleanup].
//   - [StateCancelled]: [Controller.Cancel] aborted the session; the next call is
//     [Controller.Finalize] followed by [Controller.Cleanup].
//   - [StateCancelling]: [Controller.Cancel] is aborting the migration; it
//     advances to [StateCancelled] on success or [StateFailed] on error.
//   - [StateFailed]: the session hit an unrecoverable error; the next call is
//     [Controller.Finalize] or [Controller.Cancel], then [Controller.Cleanup].
//
// # Failure and cancellation
//
// [Controller.Cancel] aborts a session in progress. A destination that has only
// imported ([StateDestinationImported]) has no compute system to abort and moves
// straight to [StateCancelled]; any other active session passes through
// [StateCancelling] while the abort runs, reaching [StateCancelled], or
// [StateFailed] if the abort itself fails. A cancel landing during
// [StateTransferring] wins over the transfer.
//
// [StateFailed] and [StateCancelled] wind down like a completed transfer:
//
//	{StateTransferCompleted | StateFailed | StateCancelled}
//	    ── Finalize ──▶ StateFinalized ── Cleanup ──▶ StateIdle
//
// A cancelled destination is the exception: its [Controller.Finalize] is a no-op,
// so it cleans up directly from [StateCancelled].
//
// # Notifications
//
// Callers observe session progress by streaming events with
// [Controller.Subscribe]; a failed transfer is reported to subscribers rather
// than returned from [Controller.Transfer].
package migration
