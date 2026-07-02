//go:build windows && lcow

package migration

// State is the current state of a live migration session, advanced by the
// [Controller] APIs. The source and destination follow distinct paths that
// converge once the transport socket is ready, and both return to [StateIdle].
//
// Source progression:
//
//	StateIdle → StateSourcePrepared → StateSourceExported → StateSocketReady
//	  → StateTransferring → StateTransferCompleted → StateFinalized → StateIdle
//
// Destination progression:
//
//	StateIdle → StateDestinationImported → StateDestinationPrepared → StateSocketReady
//	  → StateTransferring → StateTransferCompleted → StateFinalized → StateIdle
//
// [StateSocketWaiting] is entered instead of [StateSocketReady] when
// [Controller.Transfer] runs before the socket arrives. A failed transfer
// enters [StateFailed], from which [Controller.Cancel] moves to
// [StateCancelled]; [Controller.Finalize] then [Controller.Cleanup] wind a
// canceled or completed session down to [StateIdle].
type State int32

const (
	// StateIdle indicates no migration session is active. This is the
	// initial state and the state the controller returns to after
	// [Controller.Cleanup] completes.
	StateIdle State = iota

	// StateSourcePrepared indicates [Controller.PrepareSource] has armed
	// the source-side migration. The next valid call is
	// [Controller.ExportState].
	StateSourcePrepared

	// StateSourceExported indicates the source has produced its opaque saved-state
	// envelope via [Controller.ExportState]. The next valid call is
	// [Controller.RegisterDuplicateSocket].
	StateSourceExported

	// StateDestinationImported indicates the destination has rehydrated the source
	// snapshot via [Controller.ImportState]. The next valid calls are
	// [Controller.PatchResourcePaths] (one per container) followed by
	// [Controller.PrepareDestination].
	StateDestinationImported

	// StateDestinationPrepared indicates the destination has materialized the
	// HCS compute system via [Controller.PrepareDestination]. The next
	// valid call is [Controller.RegisterDuplicateSocket].
	StateDestinationPrepared

	// StateSocketWaiting indicates [Controller.Transfer] has claimed the
	// session and a single goroutine is driving it, waiting for the duplicate
	// socket if it is not yet ready.
	StateSocketWaiting

	// StateSocketReady indicates the duplicated migration transport socket
	// has been adopted via [Controller.RegisterDuplicateSocket]. The next
	// valid call is [Controller.Transfer].
	StateSocketReady

	// StateTransferring indicates [Controller.Transfer] has kicked off the
	// memory transfer. The controller automatically transitions to
	// [StateTransferCompleted] or [StateFailed] when the transfer ends.
	StateTransferring

	// StateTransferCompleted indicates the memory transfer finished
	// successfully. The next valid call is [Controller.Finalize].
	StateTransferCompleted

	// StateFinalized indicates [Controller.Finalize] has applied the final
	// operation. The next valid call is [Controller.Cleanup].
	StateFinalized

	// StateCancelled indicates the session was aborted via [Controller.Cancel].
	// The next valid call is [Controller.Finalize], followed by
	// [Controller.Cleanup] to return the controller to [StateIdle].
	StateCancelled

	// StateFailed indicates the memory transfer failed. The next valid call is
	// [Controller.Cancel] to abort the session.
	StateFailed
)

// String returns a human-readable representation of the migration State.
func (s State) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateSourcePrepared:
		return "SourcePrepared"
	case StateSourceExported:
		return "SourceExported"
	case StateDestinationImported:
		return "DestinationImported"
	case StateDestinationPrepared:
		return "DestinationPrepared"
	case StateSocketWaiting:
		return "SocketWaiting"
	case StateSocketReady:
		return "SocketReady"
	case StateTransferring:
		return "Transferring"
	case StateTransferCompleted:
		return "TransferCompleted"
	case StateFinalized:
		return "Finalized"
	case StateCancelled:
		return "Cancelled"
	case StateFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}
