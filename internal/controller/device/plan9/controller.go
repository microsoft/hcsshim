//go:build windows && lcow

package plan9

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/mount"
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/share"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/sirupsen/logrus"
)

// Controller manages the full Plan9 share lifecycle — name allocation, VM
// attachment, guest mounting, and teardown. All operations are serialized
// by a single mutex.
// It is required that all callers:
//
// 1. Obtain a reservation using Reserve().
//
// 2. Use the reservation in MapToGuest() to mount the share into the guest.
//
// 3. Call UnmapFromGuest() to release the reservation and all resources.
//
// If MapToGuest() fails, the caller must call UnmapFromGuest() to release the
// reservation and all resources.
//
// If UnmapFromGuest() fails, the caller must call UnmapFromGuest() again until
// it succeeds to release the reservation and all resources.
type Controller struct {
	// mu serializes all public operations on the Controller.
	mu sync.Mutex

	// vmPlan9 is the host-side interface for adding and removing Plan9 shares.
	// Immutable after construction.
	vmPlan9 vmPlan9

	// guest is the guest-side interface for LCOW Plan9 operations.
	// Immutable after construction.
	guest guestPlan9

	// noWritableFileShares disallows adding writable Plan9 shares.
	// Immutable after construction.
	noWritableFileShares bool

	// reservations maps a reservation ID to its share host path.
	// Guarded by mu.
	reservations map[guid.GUID]*reservation

	// sharesByHostPath maps a host path to its share for fast deduplication
	// of share additions. Guarded by mu.
	sharesByHostPath map[string]*share.Share

	// nameCounter is the monotonically increasing index used to generate
	// unique share names. Guarded by mu.
	nameCounter uint64
}

// New creates a new [Controller] for managing the plan9 shares on a VM.
func New(vm vmPlan9, guest guestPlan9, noWritableFileShares bool) *Controller {
	return &Controller{
		vmPlan9:              vm,
		guest:                guest,
		noWritableFileShares: noWritableFileShares,
		reservations:         make(map[guid.GUID]*reservation),
		sharesByHostPath:     make(map[string]*share.Share),
	}
}

// Reserve reserves a reference-counted mapping entry for a Plan9 share based on
// the share host path.
//
// If an error is returned from this function, it is guaranteed that no
// reservation mapping was made and no UnmapFromGuest() call is necessary to
// clean up.
func (c *Controller) Reserve(ctx context.Context, shareConfig share.Config, mountConfig mount.Config) (guid.GUID, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Validate write-share policy before touching shared state.
	if !shareConfig.ReadOnly && c.noWritableFileShares {
		return guid.GUID{}, fmt.Errorf("adding writable Plan9 shares is denied")
	}

	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.HostPath, shareConfig.HostPath))
	log.G(ctx).Debug("reserving Plan9 share")

	// Generate a unique reservation ID.
	id, err := guid.NewV4()
	if err != nil {
		return guid.GUID{}, fmt.Errorf("generate reservation ID: %w", err)
	}

	// Check if the generated reservation ID already exists, which is extremely unlikely,
	// but we want to be certain before proceeding with share creation.
	if _, ok := c.reservations[id]; ok {
		return guid.GUID{}, fmt.Errorf("reservation ID already exists: %s", id)
	}

	// Create the reservation entry.
	res := &reservation{
		hostPath: shareConfig.HostPath,
	}

	// Check whether this host path already has an allocated share.
	existingShare, ok := c.sharesByHostPath[shareConfig.HostPath]

	// We have an existing share for this host path — reserve a mount on it for this caller.
	if ok {
		// Verify the caller is requesting the same share configuration.
		if !existingShare.Config().Equals(shareConfig) {
			return guid.GUID{}, fmt.Errorf("cannot reserve ref on share with different config")
		}

		// Set the share name.
		res.name = existingShare.Name()

		// We have a share, now reserve a mount on it.
		if _, err = existingShare.ReserveMount(ctx, mountConfig); err != nil {
			return guid.GUID{}, fmt.Errorf("reserve mount on share %s: %w", existingShare.Name(), err)
		}
	}

	// If we don't have an existing share, we need to create one and reserve a mount on it.
	if !ok {
		// No existing share for this path — allocate a new one.
		name := strconv.FormatUint(c.nameCounter, 10)
		c.nameCounter++

		// Create the Share and Mount in the reserved states.
		newShare := share.NewReserved(name, shareConfig)
		if _, err = newShare.ReserveMount(ctx, mountConfig); err != nil {
			return guid.GUID{}, fmt.Errorf("reserve mount on share %s: %w", name, err)
		}

		c.sharesByHostPath[shareConfig.HostPath] = newShare
		res.name = newShare.Name()
	}

	// Ensure our reservation is saved for all future operations.
	c.reservations[id] = res
	log.G(ctx).WithField("reservation", id).Debug("Plan9 share reserved")

	// Return the reserved guest path in addition to the reservation ID for caller convenience.
	return id, nil
}

// MapToGuest adds the reserved share to the VM and mounts it inside the guest,
// returning the guest path. It is idempotent for a reservation that is already
// fully mapped.
func (c *Controller) MapToGuest(ctx context.Context, id guid.GUID) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if the reservation exists.
	res, ok := c.reservations[id]
	if !ok {
		return "", fmt.Errorf("reservation %s not found", id)
	}

	// Validate if the host path has an associated share.
	// This should be reserved by the Reserve() call.
	existingShare, ok := c.sharesByHostPath[res.hostPath]
	if !ok {
		return "", fmt.Errorf("share for host path %s not found", res.hostPath)
	}

	log.G(ctx).WithField(logfields.HostPath, existingShare.HostPath()).Debug("mapping Plan9 share to guest")

	// Add the share to the VM (idempotent if already added).
	if err := existingShare.AddToVM(ctx, c.vmPlan9); err != nil {
		return "", fmt.Errorf("add share to VM: %w", err)
	}

	// Mount the share inside the guest.
	guestPath, err := existingShare.MountToGuest(ctx, c.guest)
	if err != nil {
		return "", fmt.Errorf("mount share to guest: %w", err)
	}

	log.G(ctx).WithField(logfields.UVMPath, guestPath).Debug("Plan9 share mapped to guest")
	return guestPath, nil
}

// UnmapFromGuest unmounts the share from the guest and, when all reservations
// for the share are released, removes the share from the VM. A failed call is
// retryable with the same reservation ID.
func (c *Controller) UnmapFromGuest(ctx context.Context, id guid.GUID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, _ = log.WithContext(ctx, logrus.WithField("res", id.String()))

	// Validate that the reservation exists before proceeding with teardown.
	res, ok := c.reservations[id]
	if !ok {
		return fmt.Errorf("reservation %s not found", id)
	}

	// Validate that the share exists before proceeding with teardown.
	// This should be reserved by the Reserve() call.
	existingShare, ok := c.sharesByHostPath[res.hostPath]
	if !ok {
		return fmt.Errorf("share for host path %s not found", res.hostPath)
	}

	log.G(ctx).WithField(logfields.HostPath, existingShare.HostPath()).Debug("unmapping Plan9 share from guest")

	// Unmount the share from the guest (ref-counted; only issues the guest
	// call when this is the last res on the share).
	if err := existingShare.UnmountFromGuest(ctx, c.guest); err != nil {
		return fmt.Errorf("unmount share from guest: %w", err)
	}

	// Remove the share from the VM when no mounts remain active.
	if err := existingShare.RemoveFromVM(ctx, c.vmPlan9); err != nil {
		return fmt.Errorf("remove share from VM: %w", err)
	}

	// If the share is now fully removed, free its entry for reuse.
	// If it's used in other reservations, it will remain until the last one is released.
	if existingShare.State() == share.StateRemoved {
		delete(c.sharesByHostPath, existingShare.HostPath())
		log.G(ctx).Debug("Plan9 share freed")
	}

	// Remove the res last so it remains available for retries if
	// any earlier step above fails.
	delete(c.reservations, id)
	log.G(ctx).Debug("Plan9 share unmapped from guest")
	return nil
}
