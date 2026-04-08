//go:build windows && lcow

package share

// State represents the current lifecycle state of a Plan9 share.
//
// The normal progression is:
//
//	StateReserved → StateAdded → StateRemoved
//
// Add failure from StateReserved transitions to [StateInvalid].
// In [StateInvalid] the share was never added to the VM, but outstanding
// mount reservations may still exist. The share remains in [StateInvalid]
// until all mount reservations have been drained via [Share.UnmountFromGuest],
// at which point [Share.RemoveFromVM] transitions it to [StateRemoved].
// A VM removal failure from StateAdded leaves the share in StateAdded
// so the caller can retry only the removal step.
//
// Full state-transition table:
//
//	Current State  │ Trigger                               │ Next State
//	───────────────┼───────────────────────────────────────┼──────────────────
//	StateReserved  │ add succeeds                          │ StateAdded
//	StateReserved  │ add fails                             │ StateInvalid
//	StateAdded     │ mount still active                    │ StateAdded (no-op)
//	StateAdded     │ removal succeeds                      │ StateRemoved
//	StateAdded     │ removal fails                         │ StateAdded
//	StateInvalid   │ RemoveFromVM (mount active)           │ StateInvalid (no-op)
//	StateInvalid   │ RemoveFromVM (no mount)               │ StateRemoved
//	StateRemoved   │ (terminal — no further transitions)   │ —
type State int

const (
	// StateReserved is the initial state; the share name has been allocated but
	// the share has not yet been added to the VM.
	StateReserved State = iota

	// StateAdded means the share has been successfully added to the VM.
	// The guest mount is driven from this state.
	StateAdded

	// StateInvalid means the VM-side add failed. The share was never
	// established on the VM, but outstanding mount reservations may still
	// need to be drained. Once all mounts are released,
	// [Share.RemoveFromVM] transitions the share to [StateRemoved].
	StateInvalid

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
	case StateInvalid:
		return "Invalid"
	case StateRemoved:
		return "Removed"
	default:
		return "Unknown"
	}
}
