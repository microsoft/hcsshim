//go:build windows && (lcow || wcow)

// Package disk manages the lifecycle of a single SCSI disk attachment to a
// Hyper-V VM, from host-side slot allocation through guest-side partition
// mounting.
//
// # Overview
//
// [Disk] is the primary type, representing one SCSI disk attached (or to be
// attached) to the VM. It tracks its own lifecycle state via [State] and
// delegates per-partition mount management to [mount.Mount].
//
// All operations on a [Disk] are expected to be ordered by the caller.
// No locking is performed at this layer.
//
// # Disk Lifecycle
//
//	┌──────────────────────┐
//	│    StateReserved     │ ← attach failure → StateDetached (not retriable)
//	└──────────┬───────────┘
//	           │ disk added to VM SCSI bus
//	           ▼
//	┌──────────────────────┐
//	│    StateAttached     │
//	└──────────┬───────────┘
//	  (partition mounts driven here)
//	           │ all partitions released;
//	           │ SCSI device ejected from guest
//	           ▼
//	┌──────────────────────┐
//	│    StateEjected      │ ← stays here on VM removal failure (retriable)
//	└──────────┬───────────┘
//	           │ disk removed from VM SCSI bus
//	           ▼
//	┌──────────────────────┐
//	│    StateDetached     │
//	└──────────────────────┘
//	  (terminal — no further transitions)
//
// # Retry / Idempotency
//
// After a successful guest eject but a failed [Disk.DetachFromVM] VM removal,
// the disk remains in [StateEjected]. A subsequent [Disk.DetachFromVM] call
// resumes from that state and retries only the VM removal step.
//
// Attachment failure from [StateReserved] moves the disk to the terminal
// [StateDetached] state; a new [Disk] must be created to retry.
package disk
