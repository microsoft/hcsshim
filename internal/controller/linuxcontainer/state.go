//go:build windows && lcow

package linuxcontainer

// State represents the current lifecycle state of the container.
//
// Normal progression:
//
//	StateNotCreated → StateCreated → StateRunning → StateStopped
//
// Full state-transition table:
//
//	Current State    │ Trigger                                          │ Next State
//	─────────────────┼──────────────────────────────────────────────────┼────────────────
//	StateNotCreated  │ Create succeeds                                  │ StateCreated
//	StateNotCreated  │ Create fails during resource allocation or later │ StateInvalid
//	StateCreated     │ Start succeeds                                   │ StateRunning
//	StateCreated     │ Start fails                                      │ StateInvalid
//	StateRunning     │ init process exits                               │ StateStopped
//	StateStopped     │ (terminal — no further transitions)              │ —
//	StateInvalid     │ (terminal — no further transitions)              │ —
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
	default:
		return "Unknown"
	}
}
