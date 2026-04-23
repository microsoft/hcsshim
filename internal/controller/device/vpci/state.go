//go:build windows && (lcow || wcow)

package vpci

// State represents the current state of a vPCI device assignment lifecycle.
//
// The normal progression is:
//
//	StateReserved → StateAssigned → StateReady → StateRemoved+untracked
//
// [StateAssigned] is a transient state set within a single [Controller.AddToVM] call
// after the host-side HCS modify succeeds. [waitGuestDeviceReady] is then called; on
// success the device moves to [StateReady], on failure to [StateAssignedInvalid].
//
// A device transitions to [StateAssignedInvalid] when an operation partially succeeds
// and leaves the VM in an inconsistent state. This can happen in two ways:
//   - [Controller.AddToVM]: host-side assignment succeeds but guest-side notification fails.
//   - [Controller.RemoveFromVM]: the host-side remove call fails.
//
// A device in [StateAssignedInvalid] can only be cleaned up via [Controller.RemoveFromVM].
//
// Full state-transition table:
//
//	Current State        │ Trigger                                            │ Next State
//	─────────────────────┼────────────────────────────────────────────────────┼──────────────────────
//	StateReserved        │ AddToVM host-side succeeds                         │ StateAssigned
//	StateReserved        │ AddToVM host-side fails                            │ StateRemoved
//	StateReserved        │ RemoveFromVM called                                │ (untracked)
//	StateAssigned        │ waitGuestDeviceReady succeeds                      │ StateReady
//	StateAssigned        │ waitGuestDeviceReady fails                         │ StateAssignedInvalid
//	StateReady           │ AddToVM called (refCount++)                        │ StateReady
//	StateReady           │ RemoveFromVM refCount drops to 0, succeeds         │ (untracked)
//	StateReady           │ RemoveFromVM refCount drops to 0, host-side fails  │ StateAssignedInvalid
//	StateAssignedInvalid │ RemoveFromVM succeeds                              │ (untracked)
//	StateAssignedInvalid │ RemoveFromVM host-side fails                       │ StateAssignedInvalid
//	StateRemoved         │ AddToVM called                                     │ error (call RemoveFromVM)
//	StateRemoved         │ RemoveFromVM called                                │ (untracked)
type State int32

const (
	// StateReserved indicates that a VMBus GUID has been generated and the
	// device has been recorded in the Controller, but it has not yet been
	// assigned to the VM via a host-side HCS modify call.
	// This is the initial state set by [Controller.Reserve].
	StateReserved State = iota

	// StateAssigned is a transient state that indicates the host-side HCS modify
	// has succeeded but [waitGuestDeviceReady] has not yet been called/completed
	// within a single [Controller.AddToVM] invocation.
	// External callers should never observe this state.
	StateAssigned

	// StateReady indicates the device has been fully assigned to the VM:
	// the host-side HCS modify succeeded and the guest-side device is ready.
	// The reference count may be greater than one when multiple callers share
	// the same device.
	StateReady

	// StateAssignedInvalid indicates the device is in an inconsistent state due to a
	// partial failure. This state is reached in two ways:
	//   - [Controller.AddToVM]: the host-side assignment succeeded but the
	//     guest-side notification failed; the host-side assignment still exists
	//     but the guest-side device is not in a usable state.
	//   - [Controller.RemoveFromVM]: the host-side remove call failed; the
	//     host-side assignment still exists but the reference count has been
	//     decremented to zero.
	// In either case the device must be cleaned up by calling [Controller.RemoveFromVM].
	StateAssignedInvalid

	// StateRemoved indicates that no host-side VM assignment exists for this device.
	// This state is reached in two ways:
	//   - [Controller.AddToVM]: the host-side add call failed. The device is still
	//     tracked in the Controller and the caller must call [Controller.RemoveFromVM]
	//     to clean up the reservation. No further [Controller.AddToVM] calls are allowed.
	//   - [Controller.untrack]: set as a safety marker immediately before the device
	//     is deleted from the tracking maps. In this case the state is never externally
	//     observable — the device is gone from the map by the time the lock is released.
	StateRemoved
)

// String returns a human-readable string representation of the device State.
func (s State) String() string {
	switch s {
	case StateReserved:
		return "Reserved"
	case StateAssigned:
		return "Assigned"
	case StateReady:
		return "Ready"
	case StateAssignedInvalid:
		return "AssignedInvalid"
	case StateRemoved:
		return "Removed"
	default:
		return "Unknown"
	}
}
