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
	hcs "github.com/Microsoft/hcsshim/internal/hcs/v2"
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

	// reservations maps a reservation ID to its share and guest mount.
	// Guarded by mu.
	reservations map[guid.GUID]*reservation

	// sharesByHostPath groups shares by host path. Different share configurations
	// create separate shares. The same share configuration reuses an existing share,
	// where identical guest mount configurations share a reference-counted mount.
	// Guarded by mu.
	sharesByHostPath map[string]map[*share.Share]struct{}

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
		sharesByHostPath:     make(map[string]map[*share.Share]struct{}),
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
		return guid.GUID{}, fmt.Errorf("adding writable shares is denied: %w", hcs.ErrOperationDenied)
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

	// Look for a matching configuration among shares registered for this host path.
	shares := c.sharesByHostPath[shareConfig.HostPath]
	var selected *share.Share
	for existing := range shares {
		if existing.Config().Equals(shareConfig) {
			selected = existing
			break
		}
	}

	// Allocate a new share when this host path has no matching configuration.
	if selected == nil {
		name := strconv.FormatUint(c.nameCounter, 10)
		c.nameCounter++
		selected = share.NewReserved(name, shareConfig)
	}

	// Reserve the requested guest mount before registering any new share.
	guestMount, err := selected.ReserveMount(ctx, mountConfig)
	if err != nil {
		return guid.GUID{}, fmt.Errorf("reserve mount on share %s: %w", selected.Name(), err)
	}

	// Register the share under its real host path after reservation succeeds.
	if shares == nil {
		shares = make(map[*share.Share]struct{})
		c.sharesByHostPath[shareConfig.HostPath] = shares
	}
	shares[selected] = struct{}{}

	// Record the exact share and mount for subsequent mapping and cleanup.
	c.reservations[id] = &reservation{share: selected, mount: guestMount}
	log.G(ctx).WithField("reservation", id).Debug("Plan9 share reserved")

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

	// Reject remapping a reservation whose guest-mount reference was already released.
	if res.mount == nil {
		return "", fmt.Errorf("reservation %s is being released", id)
	}
	existingShare := res.share

	log.G(ctx).WithField(logfields.HostPath, existingShare.HostPath()).Debug("mapping Plan9 share to guest")

	// Add the share to the VM (idempotent if already added).
	if err := existingShare.AddToVM(ctx, c.vmPlan9); err != nil {
		return "", fmt.Errorf("add share to VM: %w", err)
	}

	// Mount the guest configuration selected by this reservation.
	guestPath, err := existingShare.MountToGuest(ctx, c.guest, res.mount)
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

	// Use the reserved share, not another configuration registered for the same host path.
	existingShare := res.share
	log.G(ctx).WithField(logfields.HostPath, existingShare.HostPath()).Debug("unmapping Plan9 share from guest")

	// Release only this caller's guest-mount reference; other mounts keep the share alive.
	if res.mount != nil {
		if err := existingShare.UnmountFromGuest(ctx, c.guest, res.mount); err != nil {
			return fmt.Errorf("unmount share from guest: %w", err)
		}
		// A host-removal retry must not release another mount reference.
		res.mount = nil
	}

	// Remove the share from the VM when no mounts remain active.
	if err := existingShare.RemoveFromVM(ctx, c.vmPlan9); err != nil {
		return fmt.Errorf("remove share from VM: %w", err)
	}

	// Remove only this share; retain the host-path entry while other variants remain.
	if existingShare.State() == share.StateRemoved {
		shares := c.sharesByHostPath[existingShare.HostPath()]
		delete(shares, existingShare)
		if len(shares) == 0 {
			delete(c.sharesByHostPath, existingShare.HostPath())
		}
		log.G(ctx).Debug("Plan9 share freed")
	}

	// Remove the res last so it remains available for retries if
	// any earlier step above fails.
	delete(c.reservations, id)
	log.G(ctx).Debug("Plan9 share unmapped from guest")
	return nil
}
