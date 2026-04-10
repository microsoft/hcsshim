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
// Full state-transition table:
//
//	Current State    │ Trigger                              │ Next State
//	─────────────────┼──────────────────────────────────────┼────────────────
//	StateNotCreated  │ Create succeeds                      │ StateCreated
//	StateCreated     │ Start succeeds                       │ StateRunning
//	StateCreated     │ Start fails / Kill / Delete          │ StateTerminated
//	StateRunning     │ process exits                        │ StateTerminated
//	StateRunning     │ Kill succeeds (signal or terminate)  │ StateTerminated
//	StateTerminated  │ (terminal — no further transitions)  │ —
type State int32

const (
	// StateNotCreated indicates the process has not been created yet.
	// This is the initial state set by [New].
	StateNotCreated State = iota

	// StateCreated indicates the process has been created but not started.
	// IO connections are established and the process spec is stored.
	StateCreated

	// StateRunning indicates the process has been started and is executing.
	StateRunning

	// StateTerminated indicates the process has exited and all cleanup is done.
	// This is a terminal state — no further transitions are possible.
	StateTerminated
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
		// StateNotCreated has no direct containerd equivalent.
		return containerdtypes.Status_UNKNOWN
	}
}
