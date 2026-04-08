//go:build windows && lcow

package mount

// State represents the current lifecycle state of a Plan9 mount inside
// the guest.
//
// The normal progression is:
//
//	StateReserved → StateMounted → StateUnmounted
//
// Mount failure from StateReserved transitions to [StateInvalid].
// In [StateInvalid] the guest mount was never established, but outstanding
// reservations may still exist. The mount remains in [StateInvalid] until
// all reservations have been drained via [Mount.UnmountFromGuest], at which
// point it transitions to [StateUnmounted].
// An unmount failure from StateMounted leaves the mount in StateMounted so
// the caller can retry.
//
// Full state-transition table:
//
//	Current State   │ Trigger                                    │ Next State
//	────────────────┼────────────────────────────────────────────┼──────────────────────
//	StateReserved   │ guest mount succeeds                       │ StateMounted
//	StateReserved   │ guest mount fails                          │ StateInvalid
//	StateReserved   │ UnmountFromGuest (refCount > 1)            │ StateReserved (ref--)
//	StateReserved   │ UnmountFromGuest (refCount == 1)           │ StateUnmounted
//	StateMounted    │ UnmountFromGuest (refCount > 1)            │ StateMounted (ref--)
//	StateMounted    │ UnmountFromGuest (refCount == 1) succeeds  │ StateUnmounted
//	StateMounted    │ UnmountFromGuest (refCount == 1) fails     │ StateMounted
//	StateInvalid    │ UnmountFromGuest (refCount > 1)            │ StateInvalid (ref--)
//	StateInvalid    │ UnmountFromGuest (refCount == 1)           │ StateUnmounted
//	StateUnmounted  │ UnmountFromGuest                           │ StateUnmounted (no-op)
//	StateUnmounted  │ (terminal — entry removed from share)      │ —
type State int

const (
	// StateReserved is the initial state; the mount entry has been created
	// but the guest mount operation has not yet been issued.
	StateReserved State = iota

	// StateMounted means the share has been successfully mounted inside
	// the guest. The guest path is valid from this state onward.
	StateMounted

	// StateInvalid means the guest mount operation failed. No guest-side
	// state was established, but outstanding reservations may still need
	// to be drained via [Mount.UnmountFromGuest]. Once all reservations
	// are released (refCount reaches zero), the mount transitions to
	// [StateUnmounted].
	StateInvalid

	// StateUnmounted means the guest has unmounted the share. This is a
	// terminal state; the entry is removed from the parent share.
	StateUnmounted
)

// String returns a human-readable name for the [State].
func (s State) String() string {
	switch s {
	case StateReserved:
		return "Reserved"
	case StateMounted:
		return "Mounted"
	case StateInvalid:
		return "Invalid"
	case StateUnmounted:
		return "Unmounted"
	default:
		return "Unknown"
	}
}
