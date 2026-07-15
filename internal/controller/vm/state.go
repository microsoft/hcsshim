//go:build windows && (lcow || wcow)

package vm

// State represents the current state of the VM lifecycle.
//
// The normal progression is:
//
//	StateNotCreated → StateCreated → StateRunning → StateTerminated
//
// If an unrecoverable error occurs during [Controller.StartVM] or
// [Controller.TerminateVM], the VM transitions to [StateInvalid] instead.
// A VM in [StateInvalid] can only be cleaned up via [Controller.TerminateVM].
//
// Live migration walks a granular, side-specific path. The source advances
// StateRunning → [StateSourceMigrationInitialized] → [StateSourceMigrationStarted];
// the destination advances StateNotCreated → [StateDestinationMigrationImported] →
// [StateDestinationMigrationCreated] → [StateDestinationMigrationPatched] →
// [StateDestinationMigrationStarted]. Both then converge: StartLiveMigrationTransfer
// reaches the shared [StateMigrationTransferCompleted], and a resume finalize reaches
// [StateMigrationFinalized] (then [Controller.Resume] → StateRunning); a stop finalize
// or [Controller.TerminateVM] reaches [StateTerminated].
//
// Full state-transition table:
//
//	Current State                        │ Trigger                          │ Next State
//	─────────────────────────────────────┼──────────────────────────────────┼─────────────────────────────────────
//	StateNotCreated                      │ CreateVM succeeds                │ StateCreated
//	StateCreated                         │ StartVM succeeds                 │ StateRunning
//	StateCreated                         │ TerminateVM succeeds             │ StateTerminated
//	StateCreated                         │ StartVM/TerminateVM fails        │ StateInvalid
//	StateRunning                         │ VM exits or TerminateVM succeeds │ StateTerminated
//	StateRunning                         │ TerminateVM fails (uvm.Close)    │ StateInvalid
//	StateRunning                         │ InitializeLiveMigrationOnSource  │ StateSourceMigrationInitialized
//	StateSourceMigrationInitialized      │ Save                             │ StateSourceMigrationInitialized
//	StateSourceMigrationInitialized      │ StartLiveMigrationOnSource       │ StateSourceMigrationStarted
//	StateSourceMigrationStarted          │ StartLiveMigrationTransfer       │ StateMigrationTransferCompleted
//	StateNotCreated                      │ Import (destination)             │ StateDestinationMigrationImported
//	StateDestinationMigrationImported    │ CreateVM (destination)           │ StateDestinationMigrationCreated
//	StateDestinationMigrationCreated     │ Patch                            │ StateDestinationMigrationPatched
//	StateDestinationMigrationPatched     │ StartWithMigrationOptions        │ StateDestinationMigrationStarted
//	StateDestinationMigrationStarted     │ StartLiveMigrationTransfer       │ StateMigrationTransferCompleted
//	StateMigrationTransferCompleted      │ FinalizeLiveMigration (Resume)   │ StateMigrationFinalized
//	StateMigrationTransferCompleted      │ FinalizeLiveMigration (Stop)     │ StateTerminated
//	StateMigrationFinalized              │ Resume                           │ StateRunning
//	(any migrating state)                │ TerminateVM                      │ StateTerminated / StateInvalid
//	StateInvalid                         │ TerminateVM called               │ StateTerminated
//	StateTerminated                      │ (terminal)                       │ —
type State int32

const (
	// StateNotCreated indicates the VM has not been created yet.
	// This is the initial state when a Controller is first instantiated via [New].
	StateNotCreated State = iota

	// StateCreated indicates the VM has been created but not yet started.
	StateCreated

	// StateRunning indicates the VM has been started and is running. The guest OS
	// is up and the Guest Compute Service (GCS) connection is established.
	StateRunning

	// StateTerminated indicates the VM has exited or been successfully terminated.
	// This is a terminal state — once reached, no further state transitions are possible.
	StateTerminated

	// StateInvalid indicates that an unrecoverable error has occurred (a failed
	// [Controller.StartVM] after the HCS VM started, or a failed uvm.Close during
	// [Controller.TerminateVM]). It can only be cleaned up via [Controller.TerminateVM].
	StateInvalid

	// StateSourceMigrationInitialized indicates the running source VM has begun an
	// outgoing migration via [Controller.InitializeLiveMigrationOnSource]. Only
	// [Controller.Save] and live-migration calls are permitted.
	StateSourceMigrationInitialized

	// StateSourceMigrationStarted indicates the source has started streaming state via
	// [Controller.StartLiveMigrationOnSource]; [Controller.StartLiveMigrationTransfer]
	// advances it to [StateMigrationTransferCompleted].
	StateSourceMigrationStarted

	// StateDestinationMigrationImported indicates the destination has been rehydrated
	// from a snapshot via [Controller.Import] but the VM does not exist yet;
	// [Controller.CreateVM] is the next step.
	StateDestinationMigrationImported

	// StateDestinationMigrationCreated indicates the destination VM has been created
	// from the snapshot but not started; [Controller.Patch] is next.
	StateDestinationMigrationCreated

	// StateDestinationMigrationPatched indicates the destination VM's disks have been
	// rebound via [Controller.Patch]; [Controller.StartWithMigrationOptions] is next.
	StateDestinationMigrationPatched

	// StateDestinationMigrationStarted indicates the destination VM is running against
	// the migration transport via [Controller.StartWithMigrationOptions], awaiting the
	// source's state; [Controller.StartLiveMigrationTransfer] advances it to
	// [StateMigrationTransferCompleted].
	StateDestinationMigrationStarted

	// StateMigrationTransferCompleted indicates the synchronous memory transfer has
	// completed on either side via [Controller.StartLiveMigrationTransfer].
	// [Controller.FinalizeLiveMigration] advances it: a resume finalize to
	// [StateMigrationFinalized], a stop finalize to [StateTerminated].
	StateMigrationTransferCompleted

	// StateMigrationFinalized indicates a resume finalize has completed on either the
	// source or the destination; [Controller.Resume] returns it to [StateRunning].
	StateMigrationFinalized
)

// String returns a human-readable string representation of the VM State.
func (s State) String() string {
	switch s {
	case StateNotCreated:
		return "NotCreated"
	case StateCreated:
		return "Created"
	case StateRunning:
		return "Running"
	case StateTerminated:
		return "Terminated"
	case StateInvalid:
		return "Invalid"
	case StateSourceMigrationInitialized:
		return "SourceMigrationInitialized"
	case StateSourceMigrationStarted:
		return "SourceMigrationStarted"
	case StateDestinationMigrationImported:
		return "DestinationMigrationImported"
	case StateDestinationMigrationCreated:
		return "DestinationMigrationCreated"
	case StateDestinationMigrationPatched:
		return "DestinationMigrationPatched"
	case StateDestinationMigrationStarted:
		return "DestinationMigrationStarted"
	case StateMigrationTransferCompleted:
		return "MigrationTransferCompleted"
	case StateMigrationFinalized:
		return "MigrationFinalized"
	default:
		return "Unknown"
	}
}
