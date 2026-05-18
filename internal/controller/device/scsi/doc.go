//go:build windows && (lcow || wcow)

// Package scsi manages the full lifecycle of SCSI disk mappings on a
// Hyper-V VM, from host-side slot allocation through guest-side mounting.
//
// # Architecture
//
// [Controller] is the primary entry point, exposing three methods:
//
//   - [Controller.Reserve]: allocates a reference-counted SCSI slot for a
//     disk + partition pair and returns a reservation ID.
//   - [Controller.MapToGuest]: attaches the disk to the VM's SCSI bus and
//     mounts the partition inside the guest.
//   - [Controller.UnmapFromGuest]: unmounts the partition from the guest and,
//     when all reservations for a disk are released, detaches the disk from
//     the VM and frees the SCSI slot.
//
// All three operations are serialized by a single mutex on the [Controller].
//
// # Usage
//
//	c := scsi.New(numControllers, vmOps, linuxGuestOps, windowsGuestOps)
//
//	// Reserve a slot (no I/O yet):
//	id, err := c.Reserve(ctx, diskConfig, mountConfig)
//
//	// Attach the disk and mount the partition:
//	guestPath, err := c.MapToGuest(ctx, id)
//
//	// Unmount and detach when done:
//	err = c.UnmapFromGuest(ctx, id)
//
// # Retry / Idempotency
//
// [Controller.MapToGuest] is idempotent for a reservation that is already
// fully mapped. [Controller.UnmapFromGuest] is retryable: if it fails
// partway through teardown, calling it again with the same reservation ID
// resumes from where the previous attempt stopped.
//
// # Layered Design
//
// The [Controller] delegates all disk-level state to [disk.Disk] and all
// partition-level state to [mount.Mount]; it only coordinates slot allocation
// and the overall call sequence.
package scsi
