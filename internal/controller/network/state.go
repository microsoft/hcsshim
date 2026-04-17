//go:build windows && (lcow || wcow)

package network

// State represents the current lifecycle state of the network for a pod.
//
// The normal progression is:
//
//	StateNotConfigured → StateConfigured → StateTornDown
//
// If an unrecoverable error occurs during [Controller.Setup], the network
// transitions to [StateInvalid] instead.
// A network in [StateInvalid] can only be cleaned up via [Controller.Teardown].
//
// Full state-transition table:
//
//	Current State       │ Trigger          │ Next State
//	────────────────────┼──────────────────┼──────────────────
//	StateNotConfigured  │ Setup succeeds   │ StateConfigured
//	StateNotConfigured  │ Setup fails      │ StateInvalid
//	StateConfigured     │ Teardown called  │ StateTornDown
//	StateInvalid        │ Teardown called  │ StateTornDown
//	StateTornDown       │ (terminal)       │ —
type State int32

const (
	// StateNotConfigured is the initial state: no namespace has been attached
	// and no NICs have been added.
	// Valid transitions:
	//   - StateNotConfigured → StateConfigured (via [Controller.Setup], on success)
	//   - StateNotConfigured → StateInvalid    (via [Controller.Setup], on failure)
	StateNotConfigured State = iota

	// StateConfigured indicates the network is fully operational: the HCN namespace
	// is attached, all endpoints are wired up, and guest-side NICs are hot-added.
	// Valid transition:
	//   - StateConfigured → StateTornDown (via [Controller.Teardown])
	StateConfigured

	// StateInvalid indicates an unrecoverable error occurred during [Controller.Setup].
	// Teardown must be called to attempt best-effort cleanup.
	// Valid transition:
	//   - StateInvalid → StateTornDown (via [Controller.Teardown])
	StateInvalid

	// StateTornDown is the terminal state reached after Teardown completes
	// (regardless of whether Setup previously succeeded or failed).
	// No further calls to Setup or Teardown are permitted.
	StateTornDown
)

// String returns a human-readable string representation of the network State.
func (s State) String() string {
	switch s {
	case StateNotConfigured:
		return "NotConfigured"
	case StateConfigured:
		return "Configured"
	case StateInvalid:
		return "Invalid"
	case StateTornDown:
		return "TornDown"
	default:
		return "Unknown"
	}
}
