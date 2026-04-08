//go:build windows && lcow

// Package mount manages the lifecycle of a single Plan9 guest-side mount
// inside a Hyper-V guest VM, from the initial reservation through mounting
// and unmounting.
//
// # Overview
//
// [Mount] is the primary type, representing one guest-side Plan9 mount.
// It tracks its own lifecycle state via [State] and supports reference
// counting so multiple callers can share the same mount.
//
// All operations on a [Mount] are expected to be ordered by the caller.
// No locking is performed at this layer.
//
// # Mount Lifecycle
//
//	┌──────────────────────┐
//	│    StateReserved     │ ← mount failure → StateInvalid
//	└──────────┬───────────┘                       │
//	           │ guest mount succeeds              │ all refs drained
//	           ▼                                   ▼
//	┌──────────────────────┐              ┌──────────────────────┐
//	│    StateMounted      │              │   StateUnmounted     │
//	└──────────┬───────────┘              └──────────────────────┘
//	           │ reference count → 0;       (terminal — entry
//	           │ guest unmount succeeds      removed from share)
//	           ▼
//	┌──────────────────────┐
//	│   StateUnmounted     │
//	└──────────────────────┘
//	  (terminal — entry removed from share)
//
// # Reference Counting
//
// Multiple callers may share a single [Mount] by calling [Mount.Reserve]
// before the mount is issued. [Mount.MountToGuest] issues the guest operation
// only once regardless of the reservation count; subsequent callers receive the
// same guest path. [Mount.UnmountFromGuest] decrements the count and only
// issues the guest unmount when it reaches zero.
package mount
