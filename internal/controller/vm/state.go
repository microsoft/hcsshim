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
// Live migration has two side-specific migrating states. The source toggles a
// running VM into [StateSourceMigrating]; the destination walks a dedicated
// path: [Controller.Import] → [StateMigratingImported], [Controller.CreateVM] →
// [StateMigratingCreated], [Controller.StartWithMigrationOptions] →
// [StateDestinationMigrating]. From either migrating state the resumed side
// returns to [StateRunning] via [Controller.Resume], while the stopped side
// reaches [StateTerminated] via a finalize Stop or a teardown
// ([Controller.TerminateVM]). The forward flow stops the source and resumes the
// destination; the reverse flow resumes the source and stops the destination.
//
// Full state-transition table:
//
//	Current State             │ Trigger                            │ Next State
//	──────────────────────────┼────────────────────────────────────┼───────────────────────
//	StateNotCreated           │ CreateVM succeeds                  │ StateCreated
//	StateCreated              │ StartVM succeeds                   │ StateRunning
//	StateCreated              │ TerminateVM succeeds               │ StateTerminated
//	StateCreated              │ StartVM fails                      │ StateInvalid
//	StateCreated              │ TerminateVM fails                  │ StateInvalid
//	StateRunning              │ VM exits or TerminateVM succeeds   │ StateTerminated
//	StateRunning              │ TerminateVM fails (uvm.Close)      │ StateInvalid
//	StateRunning              │ InitializeLiveMigrationOnSource    │ StateSourceMigrating
//	StateSourceMigrating      │ StartLiveMigrationOnSource         │ StateSourceMigrating
//	StateSourceMigrating      │ StartLiveMigrationTransfer         │ StateSourceMigrating
//	StateSourceMigrating      │ Save                               │ StateSourceMigrating
//	StateSourceMigrating      │ FinalizeLiveMigration (Resume)     │ StateSourceMigrating
//	StateSourceMigrating      │ FinalizeLiveMigration (Stop)       │ StateTerminated
//	StateSourceMigrating      │ Resume                             │ StateRunning
//	StateSourceMigrating      │ TerminateVM (abort)                │ StateTerminated
//	StateNotCreated           │ Import (destination)               │ StateMigratingImported
//	StateMigratingImported    │ CreateVM (destination)             │ StateMigratingCreated
//	StateMigratingCreated     │ Patch                              │ StateMigratingCreated
//	StateMigratingCreated     │ StartWithMigrationOptions          │ StateDestinationMigrating
//	StateMigratingImported    │ TerminateVM                        │ StateTerminated
//	StateMigratingCreated     │ TerminateVM succeeds               │ StateTerminated
//	StateMigratingCreated     │ TerminateVM fails (uvm.Close)      │ StateInvalid
//	StateDestinationMigrating │ StartLiveMigrationTransfer         │ StateDestinationMigrating
//	StateDestinationMigrating │ FinalizeLiveMigration (Resume)     │ StateDestinationMigrating
//	StateDestinationMigrating │ FinalizeLiveMigration (Stop)       │ StateTerminated
//	StateDestinationMigrating │ Resume                             │ StateRunning
//	StateDestinationMigrating │ TerminateVM (abort)                │ StateTerminated
//	StateInvalid              │ TerminateVM called                 │ StateTerminated
//	StateTerminated           │ (terminal — no further transitions)│ —
type State int32

const (
	// StateNotCreated indicates the VM has not been created yet.
	// This is the initial state when a Controller is first instantiated via [New].
	// Valid transitions: StateNotCreated → StateCreated (via [Controller.CreateVM])
	StateNotCreated State = iota

	// StateCreated indicates the VM has been created but not yet started.
	// Valid transitions:
	//   - StateCreated → StateRunning     (via [Controller.StartVM], on success)
	//   - StateCreated → StateTerminated  (via [Controller.TerminateVM], on success)
	//   - StateCreated → StateInvalid     (via [Controller.StartVM], on failure)
	StateCreated

	// StateRunning indicates the VM has been started and is running.
	// The guest OS is up and the Guest Compute Service (GCS) connection is established.
	// Valid transitions:
	//   - StateRunning → StateTerminated (VM exits naturally or [Controller.TerminateVM] succeeds)
	//   - StateRunning → StateInvalid    ([Controller.TerminateVM] fails during uvm.Close)
	//   - StateRunning → StateSourceMigrating ([Controller.InitializeLiveMigrationOnSource] succeeds)
	StateRunning

	// StateTerminated indicates the VM has exited or been successfully terminated.
	// This is a terminal state — once reached, no further state transitions are possible.
	StateTerminated

	// StateInvalid indicates that an unrecoverable error has occurred.
	// The VM transitions to this state when:
	//   - [Controller.StartVM] fails after the underlying HCS VM has already started, or
	//   - [Controller.TerminateVM] fails during uvm.Close (from [StateCreated],
	//     [StateRunning], or [StateMigratingCreated]).
	// A VM in this state can only be cleaned up by calling [Controller.TerminateVM].
	StateInvalid

	// StateSourceMigrating indicates this VM is the source of an in-progress live
	// migration. Entered from [StateRunning] via [Controller.InitializeLiveMigrationOnSource].
	// Only live-migration APIs and [Controller.Save] are permitted; [Controller.Resume]
	// returns it to [StateRunning], while a finalize Stop (forward flow) or
	// [Controller.TerminateVM] terminates it.
	StateSourceMigrating

	// StateMigratingImported indicates the destination controller has been
	// rehydrated from a snapshot via [Controller.Import] but the VM does not
	// exist yet. Only [Controller.CreateVM] (or [Controller.TerminateVM]) is
	// permitted next.
	StateMigratingImported

	// StateMigratingCreated indicates the destination VM has been created from
	// an imported snapshot but not yet started. [Controller.Patch] is valid
	// only in this state; [Controller.StartWithMigrationOptions] advances it to
	// [StateDestinationMigrating], and [Controller.TerminateVM] tears it down.
	StateMigratingCreated

	// StateDestinationMigrating indicates this VM is the destination of an
	// in-progress live migration. Entered via [Controller.StartWithMigrationOptions].
	// Only live-migration APIs are permitted; [Controller.Resume] reaches [StateRunning],
	// while a finalize Stop (reverse flow) or [Controller.TerminateVM] terminates it.
	StateDestinationMigrating
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
	case StateSourceMigrating:
		return "SourceMigrating"
	case StateMigratingImported:
		return "MigratingImported"
	case StateMigratingCreated:
		return "MigratingCreated"
	case StateDestinationMigrating:
		return "DestinationMigrating"
	default:
		return "Unknown"
	}
}
