//go:build windows && lcow

package linuxcontainer

// State represents the current lifecycle state of the container.
//
// Normal progression:
//
//	StateNotCreated → StateCreated → StateRunning → StateStopped
//
// Live migration adds two branches. On the destination, the controller is
// rehydrated via [Import] directly into [StateDestinationMigrating] and rejoins
// the table above once [Controller.Resume] binds the live VM/guest dependencies
// (→ [StateRunning]) or [Controller.AbortMigrated] discards it (→ [StateStopped]).
// On the source, [Controller.Save] freezes a running container into
// [StateSourceMigrating]; [Controller.Resume] rolls it back to [StateRunning], or
// its init process exit on source VM teardown moves it to [StateStopped].
//
// Full state-transition table:
//
//	Current State             │ Trigger                                          │ Next State
//	──────────────────────────┼──────────────────────────────────────────────────┼──────────────────────
//	StateNotCreated           │ Create succeeds                                  │ StateCreated
//	StateNotCreated           │ Create fails during resource allocation or later │ StateInvalid
//	StateCreated              │ Start succeeds                                   │ StateRunning
//	StateCreated              │ Start fails                                      │ StateInvalid
//	StateRunning              │ init process exits                               │ StateStopped
//	StateRunning              │ Save freezes the source                          │ StateSourceMigrating
//	StateStopped              │ (terminal — no further transitions)              │ —
//	StateInvalid              │ (terminal — no further transitions)              │ —
//	StateDestinationMigrating │ Resume binds the live VM, guest, and devices     │ StateRunning
//	StateDestinationMigrating │ AbortMigrated discards the import                │ StateStopped
//	StateSourceMigrating      │ Resume rolls back the migration                  │ StateRunning
//	StateSourceMigrating      │ init process exits (source VM torn down)         │ StateStopped
type State int32

const (
	// StateNotCreated indicates the container has not been created yet.
	StateNotCreated State = iota

	// StateCreated indicates the container has been created but not started.
	StateCreated

	// StateRunning indicates the container has been started and is running.
	StateRunning

	// StateStopped indicates the container's init process has exited and
	// all host-side resources have been released.
	StateStopped

	// StateInvalid indicates the container entered an unrecoverable failure
	// during Create or Start.
	StateInvalid

	// StateDestinationMigrating indicates a container rehydrated from a snapshot
	// on the destination, awaiting Resume (→ StateRunning) or AbortMigrated (→ StateStopped).
	StateDestinationMigrating

	// StateSourceMigrating indicates a running container frozen by Save on the
	// source, awaiting Resume (→ StateRunning) or its VM teardown (→ StateStopped).
	StateSourceMigrating
)

// String returns a human-readable representation of the container State.
func (s State) String() string {
	switch s {
	case StateNotCreated:
		return "NotCreated"
	case StateCreated:
		return "Created"
	case StateRunning:
		return "Running"
	case StateStopped:
		return "Stopped"
	case StateInvalid:
		return "Invalid"
	case StateDestinationMigrating:
		return "DestinationMigrating"
	case StateSourceMigrating:
		return "SourceMigrating"
	default:
		return "Unknown"
	}
}
