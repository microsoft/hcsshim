//go:build windows

package vpci

// State represents the current state of a vPCI device assignment lifecycle.
//
// The normal progression is:
//
//	StateReserved → StateAssigned → StateRemoved
//
// A device transitions to [StateInvalid] when an operation partially succeeds
// and leaves the VM in an inconsistent state. This can happen in two ways:
//   - [Controller.AddToVM]: host-side assignment succeeds but guest-side notification fails.
//   - [Controller.RemoveFromVM]: the host-side remove call fails.
//
// A device in [StateInvalid] can only be cleaned up via [Controller.RemoveFromVM].
//
// Full state-transition table:
//
//	Current State │ Trigger                                               │ Next State
//	──────────────┼───────────────────────────────────────────────────────┼──────────────────
//	StateReserved │ AddToVM succeeds                                      │ StateAssigned
//	StateReserved │ RemoveFromVM called                                   │ StateRemoved
//	StateAssigned │ RemoveFromVM refCount drops to 0, succeeds            │ StateRemoved
//	StateAssigned │ AddToVM (host succeeded, guest-side fails)            │ StateInvalid
//	StateAssigned │ RemoveFromVM refCount drops to 0, host-side fails     │ StateInvalid
//	StateInvalid  │ RemoveFromVM succeeds                                 │ StateRemoved
//	StateInvalid  │ RemoveFromVM host-side fails                          │ StateInvalid
//	StateRemoved  │ (terminal — no further transitions)                   │ —
type State int32

const (
	// StateReserved indicates that a VMBus GUID has been generated and the
	// device has been recorded in the Controller, but it has not yet been
	// assigned to the VM via a host-side HCS modify call.
	// This is the initial state set by [Controller.Reserve].
	StateReserved State = iota

	// StateAssigned indicates the device has been assigned to the VM
	// (host-side HCS modify succeeded and guest-side notification succeeded).
	// The reference count may be greater than one when multiple callers share
	// the same device.
	StateAssigned

	// StateInvalid indicates the device is in an inconsistent state due to a
	// partial failure.
	// The device must be cleaned up by calling [Controller.RemoveFromVM].
	StateInvalid

	// StateRemoved indicates the device has been fully removed from the VM
	// and is no longer tracked by the Controller.
	// This is a terminal state — once reached, no further state transitions
	// are possible.
	StateRemoved
)

// String returns a human-readable string representation of the device State.
func (s State) String() string {
	switch s {
	case StateReserved:
		return "Reserved"
	case StateAssigned:
		return "Assigned"
	case StateInvalid:
		return "Invalid"
	case StateRemoved:
		return "Removed"
	default:
		return "Unknown"
	}
}
