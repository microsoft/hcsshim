//go:build windows

package share

// State represents the current lifecycle state of a Plan9 share.
//
// The normal progression is:
//
//	StateReserved → StateAdded → StateRemoved
//
// Add failure from StateReserved transitions directly to the
// terminal StateRemoved state; create a new [Share] to retry.
// A VM removal failure from StateAdded leaves the share in StateAdded
// so the caller can retry only the removal step.
//
// Full state-transition table:
//
//	Current State  │ Trigger                               │ Next State
//	───────────────┼───────────────────────────────────────┼──────────────────
//	StateReserved  │ add succeeds                          │ StateAdded
//	StateReserved  │ add fails                             │ StateRemoved
//	StateAdded     │ mount still active                    │ StateAdded (no-op)
//	StateAdded     │ removal succeeds                      │ StateRemoved
//	StateAdded     │ removal fails                         │ StateAdded
//	StateRemoved   │ (terminal — no further transitions)   │ —
type State int

const (
	// StateReserved is the initial state; the share name has been allocated but
	// the share has not yet been added to the VM.
	StateReserved State = iota

	// StateAdded means the share has been successfully added to the VM.
	// The guest mount is driven from this state.
	StateAdded

	// StateRemoved means the share has been fully removed from the VM.
	// This is a terminal state.
	StateRemoved
)

// String returns a human-readable name for the [State].
func (s State) String() string {
	switch s {
	case StateReserved:
		return "Reserved"
	case StateAdded:
		return "Added"
	case StateRemoved:
		return "Removed"
	default:
		return "Unknown"
	}
}
