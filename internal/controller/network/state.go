//go:build windows && (lcow || wcow)

package network

// State represents the current lifecycle state of the network for a pod.
//
// The normal (live-creation) progression is:
//
//	StateNotConfigured → StateConfigured → StateTornDown
//
// If an unrecoverable error occurs during [Controller.Setup], the network
// transitions to [StateInvalid] instead.
// A network in [StateInvalid] can only be cleaned up via [Controller.Teardown].
//
// Live migration adds two branches. On the destination, [Import] rehydrates the
// controller into [StateDestinationMigrating] until [Controller.Resume] binds the
// live host/guest interfaces (→ [StateConfigured]) or [Controller.Teardown] aborts
// it (→ [StateTornDown]). On the source, [Controller.Save] freezes a configured
// network into [StateSourceMigrating]; [Controller.Resume] rolls it back
// (→ [StateConfigured]) or [Controller.Teardown] tears it down (→ [StateTornDown]).
//
// Full state-transition table:
//
//	Current State             │ Trigger          │ Next State
//	──────────────────────────┼──────────────────┼──────────────────────
//	StateNotConfigured        │ Setup succeeds   │ StateConfigured
//	StateNotConfigured        │ Setup fails      │ StateInvalid
//	StateConfigured           │ Save freezes src │ StateSourceMigrating
//	StateConfigured           │ Teardown called  │ StateTornDown
//	StateInvalid              │ Teardown called  │ StateTornDown
//	StateDestinationMigrating │ Resume called    │ StateConfigured
//	StateDestinationMigrating │ Teardown called  │ StateTornDown
//	StateSourceMigrating      │ Resume called    │ StateConfigured
//	StateSourceMigrating      │ Teardown called  │ StateTornDown
//	StateTornDown             │ (terminal)       │ —
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

	// StateDestinationMigrating indicates a controller rehydrated from a snapshot
	// on the destination, awaiting Resume (→ StateConfigured) or Teardown (→ StateTornDown).
	StateDestinationMigrating

	// StateSourceMigrating indicates a configured network frozen by Save on the
	// source, awaiting Resume (→ StateConfigured) or Teardown (→ StateTornDown).
	StateSourceMigrating
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
	case StateDestinationMigrating:
		return "DestinationMigrating"
	case StateSourceMigrating:
		return "SourceMigrating"
	default:
		return "Unknown"
	}
}
