//go:build windows && lcow

// Package plan9 manages the full lifecycle of Plan9 share mappings on a
// Hyper-V VM, from host-side name allocation through guest-side mounting.
//
// # Architecture
//
// [Controller] is the primary entry point, exposing three methods:
//
//   - [Controller.Reserve]: allocates a reference-counted Plan9 share for a
//     host path and returns a reservation ID.
//   - [Controller.MapToGuest]: adds the share to the VM and mounts it
//     inside the guest.
//   - [Controller.UnmapFromGuest]: unmounts the share from the guest and,
//     when all reservations for a share are released, removes the share from
//     the VM.
//
// All three operations are serialized by a single mutex on the [Controller].
//
// # Usage
//
//	c := plan9.New(vmOps, linuxGuestOps, noWritableFileShares)
//
//	// Reserve a share (no I/O yet):
//	id, err := c.Reserve(ctx, shareConfig, mountConfig)
//
//	// Add the share and mount in the guest:
//	guestPath, err := c.MapToGuest(ctx, id)
//
//	// Unmount and remove when done:
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
// The [Controller] delegates all share-level state to [share.Share] and all
// mount-level state to [mount.Mount]; it only coordinates name allocation
// and the overall call sequence.
package plan9
