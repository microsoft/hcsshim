//go:build windows && (lcow || wcow)

package process

import (
	containerdtypes "github.com/containerd/containerd/api/types/task"
)

// State represents the current state of the process lifecycle.
//
// The normal progression is:
//
//	StateNotCreated → StateCreated → StateRunning → StateTerminated
//
// Live migration adds two branches. On the destination, a process is restored
// directly into StateDestinationMigrating and rejoins the progression once
// resumed (StateRunning) or aborted (StateTerminated). On the source, Save
// freezes a running process into StateSourceMigrating; resume rolls it back to
// StateRunning, or its exit (when the source VM is torn down) terminates it.
//
// Full state-transition table:
//
//	Current State             │ Trigger                      │ Next State
//	──────────────────────────┼──────────────────────────────┼─────────────────────
//	StateNotCreated           │ create succeeds              │ StateCreated
//	StateNotCreated           │ kill                         │ StateTerminated
//	StateCreated              │ start succeeds               │ StateRunning
//	StateCreated              │ start fails / kill / delete  │ StateTerminated
//	StateRunning              │ process exits (incl. kill)   │ StateTerminated
//	StateRunning              │ Save freezes the source      │ StateSourceMigrating
//	StateDestinationMigrating │ resume succeeds              │ StateRunning
//	StateDestinationMigrating │ migration aborted            │ StateTerminated
//	StateSourceMigrating      │ resume rolls back            │ StateRunning
//	StateSourceMigrating      │ process exits (VM torn down) │ StateTerminated
//	StateTerminated           │ terminal — no transitions    │ —
type State int32

const (
	// StateNotCreated indicates the process has not been created yet.
	// This is the initial state for a newly constructed process.
	StateNotCreated State = iota

	// StateCreated indicates the process has been created but not started.
	// IO connections are established and the process spec is stored.
	StateCreated

	// StateRunning indicates the process has been started and is executing.
	StateRunning

	// StateTerminated indicates the process has exited and all cleanup is done.
	// This is a terminal state — no further transitions are possible.
	StateTerminated

	// StateDestinationMigrating indicates a process restored from a snapshot on
	// the destination, awaiting resume (→ StateRunning) or abort (→ StateTerminated).
	StateDestinationMigrating

	// StateSourceMigrating indicates a running process frozen by Save on the
	// source, awaiting resume (→ StateRunning) or its exit (→ StateTerminated).
	StateSourceMigrating
)

// String returns a human-readable representation of the State.
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
	case StateDestinationMigrating:
		return "DestinationMigrating"
	case StateSourceMigrating:
		return "SourceMigrating"
	default:
		return "Unknown"
	}
}

// ContainerdStatus converts the State into the equivalent containerd task Status.
func (s State) ContainerdStatus() containerdtypes.Status {
	switch s {
	case StateCreated:
		return containerdtypes.Status_CREATED
	case StateRunning:
		return containerdtypes.Status_RUNNING
	case StateTerminated:
		return containerdtypes.Status_STOPPED
	default:
		// StateNotCreated and the migrating states have no direct containerd equivalent.
		return containerdtypes.Status_UNKNOWN
	}
}
