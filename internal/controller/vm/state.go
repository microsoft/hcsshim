//go:build windows

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
// Full state-transition table:
//
//	Current State    │ Trigger                            │ Next State
//	─────────────────┼────────────────────────────────────┼─────────────────
//	StateNotCreated  │ CreateVM succeeds                  │ StateCreated
//	StateCreated     │ StartVM succeeds                   │ StateRunning
//	StateCreated     │ TerminateVM succeeds               │ StateTerminated
//	StateCreated     │ StartVM fails                      │ StateInvalid
//	StateCreated     │ TerminateVM fails                  │ StateInvalid
//	StateRunning     │ VM exits or TerminateVM succeeds   │ StateTerminated
//	StateRunning     │ TerminateVM fails (uvm.Close)      │ StateInvalid
//	StateInvalid     │ TerminateVM called                 │ StateTerminated
//	StateTerminated  │ (terminal — no further transitions)│ —
type State int32

const (
	// StateNotCreated indicates the VM has not been created yet.
	// This is the initial state when a Controller is first instantiated via [NewController].
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
	StateRunning

	// StateTerminated indicates the VM has exited or been successfully terminated.
	// This is a terminal state — once reached, no further state transitions are possible.
	StateTerminated

	// StateInvalid indicates that an unrecoverable error has occurred.
	// The VM transitions to this state when:
	//   - [Controller.StartVM] fails after the underlying HCS VM has already started, or
	//   - [Controller.TerminateVM] fails during uvm.Close (from either [StateCreated] or [StateRunning]).
	// A VM in this state can only be cleaned up by calling [Controller.TerminateVM].
	StateInvalid
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
	default:
		return "Unknown"
	}
}
