//go:build windows

package scsi

import (
	"context"
	"fmt"
	"sync"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/disk"
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/sirupsen/logrus"
)

// Controller manages the full SCSI disk lifecycle — slot allocation, VM
// attachment, guest mounting, and teardown — across one or more controllers
// on a Hyper-V VM. All operations are serialized by a single mutex.
// It is required that all callers:
//
// 1. Obtain a reservation using Reserve().
//
// 2. Use the reservation to MapToGuest() to ensure resource availability.
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

	// vm is the host-side interface for adding and removing SCSI disks.
	// Immutable after construction.
	vm VMSCSIOps
	// guest is the guest-side interface for SCSI operations.
	// Immutable after construction.
	guest GuestSCSIOps

	// reservations maps a reservation ID to its disk slot and partition.
	// Guarded by mu.
	reservations map[guid.GUID]*reservation

	// disksByPath maps a host disk path to its controllerSlots index for
	// fast deduplication of disk attachments. Guarded by mu.
	disksByPath map[string]int

	// controllerSlots tracks all disk slots across all SCSI controllers.
	// A nil entry means the slot is free for allocation.
	//
	// Index layout:
	//   ControllerID = index / numLUNsPerController
	//   LUN          = index % numLUNsPerController
	controllerSlots []*disk.Disk
}

// New creates a new [Controller] for the given number of SCSI controllers and
// host/guest operation interfaces.
func New(numControllers int, vm VMSCSIOps, guest GuestSCSIOps) *Controller {
	return &Controller{
		vm:              vm,
		guest:           guest,
		reservations:    make(map[guid.GUID]*reservation),
		disksByPath:     make(map[string]int),
		controllerSlots: make([]*disk.Disk, numControllers*numLUNsPerController),
	}
}

// ReserveForRootfs reserves a specific controller and lun location for the
// rootfs. This is required to ensure the rootfs is always at a known location
// and that location is not used for any other disk. This should only be called
// once per controller and lun location, and must be called before any calls to
// Reserve() to ensure the rootfs reservation is not evicted by a dynamic
// reservation.
func (c *Controller) ReserveForRootfs(ctx context.Context, controller, lun uint) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	slot := int(controller*numLUNsPerController + lun)
	if slot >= len(c.controllerSlots) {
		return fmt.Errorf("invalid controller %d or lun %d", controller, lun)
	}
	if c.controllerSlots[slot] != nil {
		return fmt.Errorf("slot for controller %d and lun %d is already reserved", controller, lun)
	}
	c.controllerSlots[slot] = disk.NewReserved(controller, lun, disk.Config{})
	return nil
}

// Reserve reserves a referenced counted mapping entry for a SCSI attachment based on
// the SCSI disk path, and partition number.
//
// If an error is returned from this function, it is guaranteed that no
// reservation mapping was made and no UnmapFromGuest() call is necessary to
// clean up.
func (c *Controller) Reserve(ctx context.Context, diskConfig disk.Config, mountConfig mount.Config) (guid.GUID, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, _ = log.WithContext(ctx, logrus.WithFields(logrus.Fields{
		logfields.HostPath:  diskConfig.HostPath,
		logfields.Partition: mountConfig.Partition,
	}))

	log.G(ctx).Debug("reserving SCSI slot")

	// Generate a unique reservation ID.
	id, err := guid.NewV4()
	if err != nil {
		return guid.GUID{}, fmt.Errorf("generate reservation ID: %w", err)
	}
	if _, ok := c.reservations[id]; ok {
		return guid.GUID{}, fmt.Errorf("reservation ID already exists: %s", id)
	}

	// Create the reservation entry.
	r := &reservation{
		controllerSlot: -1,
		partition:      mountConfig.Partition,
	}

	// Check whether this disk path already has an allocated slot.
	if slot, ok := c.disksByPath[diskConfig.HostPath]; ok {
		r.controllerSlot = slot // Update our reservation where the dsk is.
		existingDisk := c.controllerSlots[slot]

		// Verify the caller is requesting the same disk configuration.
		if !existingDisk.Config().Equals(diskConfig) {
			return guid.GUID{}, fmt.Errorf("cannot reserve ref on disk with different config")
		}

		// We at least have a dsk, now determine if we have a mount for this
		// partition.
		if _, err := existingDisk.ReservePartition(ctx, mountConfig); err != nil {
			return guid.GUID{}, fmt.Errorf("reserve partition %d: %w", mountConfig.Partition, err)
		}
	} else {
		// No existing slot for this path — find a free one.
		nextSlot := -1
		for i, d := range c.controllerSlots {
			if d == nil {
				nextSlot = i
				break
			}
		}
		if nextSlot == -1 {
			return guid.GUID{}, fmt.Errorf("no available SCSI slots")
		}

		// Create the Disk and Partition Mount in the reserved states.
		controller := uint(nextSlot / numLUNsPerController)
		lun := uint(nextSlot % numLUNsPerController)
		newDisk := disk.NewReserved(controller, lun, diskConfig)
		if _, err := newDisk.ReservePartition(ctx, mountConfig); err != nil {
			return guid.GUID{}, fmt.Errorf("reserve partition %d: %w", mountConfig.Partition, err)
		}
		c.controllerSlots[controller*numLUNsPerController+lun] = newDisk
		c.disksByPath[diskConfig.HostPath] = nextSlot
		r.controllerSlot = nextSlot
	}

	// Ensure our reservation is saved for all future operations.
	c.reservations[id] = r
	log.G(ctx).WithField("reservation", id).Debug("SCSI slot reserved")
	return id, nil
}

// MapToGuest attaches the reserved disk to the VM and mounts its partition
// inside the guest, returning the guest path. It is idempotent for a
// reservation that is already fully mapped.
func (c *Controller) MapToGuest(ctx context.Context, id guid.GUID) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	r, ok := c.reservations[id]
	if !ok {
		return "", fmt.Errorf("reservation %s not found", id)
	}

	existingDisk := c.controllerSlots[r.controllerSlot]

	log.G(ctx).WithFields(logrus.Fields{
		logfields.HostPath:  existingDisk.HostPath(),
		logfields.Partition: r.partition,
	}).Debug("mapping SCSI disk to guest")

	// Attach the disk to the VM's SCSI bus (idempotent if already attached).
	if err := existingDisk.AttachToVM(ctx, c.vm); err != nil {
		return "", fmt.Errorf("attach disk to VM: %w", err)
	}

	// Mount the partition inside the guest.
	guestPath, err := existingDisk.MountPartitionToGuest(ctx, r.partition, c.guest)
	if err != nil {
		return "", fmt.Errorf("mount partition %d to guest: %w", r.partition, err)
	}

	log.G(ctx).WithField(logfields.UVMPath, guestPath).Debug("SCSI disk mapped to guest")
	return guestPath, nil
}

// UnmapFromGuest unmounts the partition from the guest and, when all
// reservations for a disk are released, detaches the disk from the VM and
// frees the SCSI slot. A failed call is retryable with the same reservation ID.
func (c *Controller) UnmapFromGuest(ctx context.Context, id guid.GUID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, _ = log.WithContext(ctx, logrus.WithField("reservation", id.String()))

	r, ok := c.reservations[id]
	if !ok {
		return fmt.Errorf("reservation %s not found", id)
	}

	existingDisk := c.controllerSlots[r.controllerSlot]

	log.G(ctx).WithFields(logrus.Fields{
		logfields.HostPath:  existingDisk.HostPath(),
		logfields.Partition: r.partition,
	}).Debug("unmapping SCSI disk from guest")

	// Unmount the partition from the guest (ref-counted; only issues the
	// guest call when this is the last reservation on the partition).
	if err := existingDisk.UnmountPartitionFromGuest(ctx, r.partition, c.guest); err != nil {
		return fmt.Errorf("unmount partition %d from guest: %w", r.partition, err)
	}

	// Detach the disk from the VM when no partitions remain active.
	if err := existingDisk.DetachFromVM(ctx, c.vm, c.guest); err != nil {
		return fmt.Errorf("detach disk from VM: %w", err)
	}

	// If the disk is now fully detached, free its slot for reuse.
	if existingDisk.State() == disk.StateDetached {
		delete(c.disksByPath, existingDisk.HostPath())
		c.controllerSlots[r.controllerSlot] = nil
		log.G(ctx).Debug("SCSI slot freed")
	}

	// Remove the reservation last so it remains available for retries if
	// any earlier step above fails.
	delete(c.reservations, id)
	log.G(ctx).Debug("SCSI disk unmapped from guest")
	return nil
}
