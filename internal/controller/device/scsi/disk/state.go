//go:build windows

package disk

// State represents the current lifecycle state of a SCSI disk attachment.
//
// The normal progression is:
//
//	StateReserved → StateAttached → StateEjected → StateDetached
//
// Attachment failure from StateReserved transitions directly to the
// terminal StateDetached state; create a new [Disk] to retry.
// A VM removal failure from StateEjected leaves the disk in StateEjected
// so the caller can retry only the removal step.
//
// Full state-transition table:
//
//	Current State  │ Trigger                               │ Next State
//	───────────────┼───────────────────────────────────────┼──────────────────
//	StateReserved  │ attach succeeds                       │ StateAttached
//	StateReserved  │ attach fails                          │ StateDetached
//	StateAttached  │ active mounts remain                  │ StateAttached (no-op)
//	StateAttached  │ eject succeeds (or no-op for WCOW)    │ StateEjected
//	StateAttached  │ eject fails                           │ StateAttached
//	StateEjected   │ VM removal succeeds                   │ StateDetached
//	StateEjected   │ VM removal fails                      │ StateEjected
//	StateDetached  │ (terminal — no further transitions)   │ —
type State int

const (
	// StateReserved is the initial state; the slot has been allocated but
	// the disk has not yet been added to the VM's SCSI bus.
	StateReserved State = iota

	// StateAttached means the disk has been successfully added to the
	// VM's SCSI bus. Partition mounts are driven from this state.
	StateAttached

	// StateEjected means the SCSI device has been unplugged from the
	// guest but the disk has not yet been removed from the VM's SCSI bus.
	// This intermediate state makes VM removal retriable independently of
	// the guest eject step.
	StateEjected

	// StateDetached means the disk has been fully removed from the VM's
	// SCSI bus. This is a terminal state.
	StateDetached
)

// String returns a human-readable name for the [State].
func (s State) String() string {
	switch s {
	case StateReserved:
		return "Reserved"
	case StateAttached:
		return "Attached"
	case StateEjected:
		return "Ejected"
	case StateDetached:
		return "Detached"
	default:
		return "Unknown"
	}
}
