//go:build windows && lcow

// Package share manages the lifecycle of a single Plan9 share attached to a
// Hyper-V VM, from host-side name allocation through guest-side mounting.
//
// # Overview
//
// [Share] is the primary type, representing one Plan9 share attached (or to be
// attached) to the VM. It tracks its own lifecycle state via [State] and
// delegates guest mount management to [mount.Mount].
//
// All operations on a [Share] are expected to be ordered by the caller.
// No locking is performed at this layer.
//
// # Share Lifecycle
//
//	┌──────────────────────┐
//	│    StateReserved     │ ← add failure → StateInvalid
//	└──────────┬───────────┘
//	           │ share added to VM
//	           ▼
//	┌──────────────────────┐
//	│     StateAdded       │
//	└──────────┬───────────┘
//	  (guest mount driven here)
//	           │ mount released;
//	           │ share removed from VM
//	           ▼
//	┌──────────────────────┐
//	│    StateRemoved      │
//	└──────────────────────┘
//	  (terminal — entry removed from controller)
//
//	┌──────────────────────┐
//	│    StateInvalid      │ ← add failed; share never on VM
//	└──────────┬───────────┘
//	           │ all mount reservations drained
//	           │ via UnmountFromGuest + RemoveFromVM
//	           ▼
//	┌──────────────────────┐
//	│    StateRemoved      │
//	└──────────────────────┘
//	  (terminal — entry removed from controller)
package share
