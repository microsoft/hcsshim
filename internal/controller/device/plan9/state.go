//go:build windows && !wcow

package plan9

// shareState represents the current state of a Plan9 share's lifecycle.
//
// The normal progression is:
//
//	sharePending → shareAdded → shareRemoved
//
// If AddPlan9 fails, the owning goroutine moves the share to
// shareInvalid and records the error in [shareEntry.stateErr]. Other goroutines
// waiting on the same entry observe the invalid state and receive the original error.
// The entry is removed from the map immediately after the transition.
//
// Full state-transition table:
//
//	Current State  │ Trigger                   │ Next State
//	───────────────┼───────────────────────────┼─────────────────────────────
//	sharePending   │ AddPlan9 succeeds          │ shareAdded
//	sharePending   │ AddPlan9 fails             │ shareInvalid
//	shareAdded     │ RemovePlan9 succeeds       │ shareRemoved
//	shareRemoved   │ (terminal — no transitions)│ —
//	shareInvalid   │ entry removed from map     │ —
type shareState int

const (
	// sharePending is the initial state; AddPlan9 has not yet completed.
	sharePending shareState = iota

	// shareAdded means AddPlan9 succeeded; the share is live on the VM.
	shareAdded

	// shareRemoved means RemovePlan9 succeeded. This is a terminal state.
	shareRemoved

	// shareInvalid means AddPlan9 failed.
	shareInvalid
)

// String returns a human-readable name for the [shareState].
func (s shareState) String() string {
	switch s {
	case sharePending:
		return "Pending"
	case shareAdded:
		return "Added"
	case shareRemoved:
		return "Removed"
	case shareInvalid:
		return "Invalid"
	default:
		return "Unknown"
	}
}
