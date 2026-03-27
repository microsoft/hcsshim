//go:build windows

package mount

// State represents the current lifecycle state of a partition mount inside
// the guest.
//
// The normal progression is:
//
//	StateReserved → StateMounted → StateUnmounted
//
// Mount failure from StateReserved transitions directly to the terminal
// StateUnmounted state; the entry is then removed by the parent [disk.Disk].
// An unmount failure from StateMounted leaves the mount in StateMounted so
// the caller can retry.
//
// Full state-transition table:
//
//	Current State   │ Trigger                                    │ Next State
//	────────────────┼────────────────────────────────────────────┼──────────────────────
//	StateReserved   │ guest mount succeeds                       │ StateMounted
//	StateReserved   │ guest mount fails                          │ StateUnmounted
//	StateReserved   │ UnmountFromGuest (any refCount)            │ StateReserved (ref--)
//	StateMounted    │ UnmountFromGuest (refCount > 1)            │ StateMounted (ref--)
//	StateMounted    │ UnmountFromGuest (refCount == 1) succeeds  │ StateUnmounted
//	StateMounted    │ UnmountFromGuest (refCount == 1) fails     │ StateMounted
//	StateUnmounted  │ (terminal — entry removed from disk)       │ —
type State int

const (
	// StateReserved is the initial state; the mount entry has been created
	// but the guest mount operation has not yet been issued.
	StateReserved State = iota

	// StateMounted means the partition has been successfully mounted inside
	// the guest. The guest path is valid from this state onward.
	StateMounted

	// StateUnmounted means the guest has unmounted the partition. This is a
	// terminal state; the entry is removed from the parent disk.
	StateUnmounted
)

// String returns a human-readable name for the [State].
func (s State) String() string {
	switch s {
	case StateReserved:
		return "Reserved"
	case StateMounted:
		return "Mounted"
	case StateUnmounted:
		return "Unmounted"
	default:
		return "Unknown"
	}
}
