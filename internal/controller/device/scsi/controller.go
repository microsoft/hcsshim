//go:build windows

package scsi

import (
	"context"
	"fmt"
	"sync"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/disk"
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"

	"github.com/google/uuid"
)

type VMSCSIOps interface {
	disk.VMSCSIAdder
	disk.VMSCSIRemover
}

type LinuxGuestSCSIOps interface {
	mount.LinuxGuestSCSIMounter
	mount.LinuxGuestSCSIUnmounter
	disk.LinuxGuestSCSIEjector
}

type WindowsGuestSCSIOps interface {
	mount.WindowsGuestSCSIMounter
	mount.WindowsGuestSCSIUnmounter
}

// numLUNsPerController is the maximum number of LUNs per controller, fixed by Hyper-V.
const numLUNsPerController = 64

// The controller manages all SCSI attached devices and guest mounted
// directories.
//
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
	vm     VMSCSIOps
	lGuest LinuxGuestSCSIOps
	wGuest WindowsGuestSCSIOps

	mu sync.Mutex

	// Every call to Reserve gets a unique reservation ID which holds pointers
	// to its controllerSlot for the disk and its partition for the mount.
	reservations map[uuid.UUID]*reservation

	// For fast lookup we keep a hostPath to controllerSlot mapping for all
	// allocated disks.
	disksByPath map[string]int

	// Tracks all allocated and unallocated available slots on the SCSI
	// controllers.
	//
	// NumControllers == len(controllerSlots) / numLUNsPerController
	// ControllerID == index / numLUNsPerController
	// LunID == index % numLUNsPerController
	controllerSlots []*disk.Disk
}

func New(numControllers int, vm VMSCSIOps, lGuest LinuxGuestSCSIOps, wGuest WindowsGuestSCSIOps) *Controller {
	return &Controller{
		vm:              vm,
		lGuest:          lGuest,
		wGuest:          wGuest,
		mu:              sync.Mutex{},
		reservations:    make(map[uuid.UUID]*reservation),
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
	c.controllerSlots[slot] = disk.NewReserved(controller, lun, disk.DiskConfig{})
	return nil
}

// Reserves a referenced counted mapping entry for a SCSI attachment based on
// the SCSI disk path, and partition number.
//
// If an error is returned from this function, it is guaranteed that no
// reservation mapping was made and no UnmapFromGuest() call is necessary to
// clean up.
func (c *Controller) Reserve(ctx context.Context, diskConfig disk.DiskConfig, mountConfig mount.MountConfig) (uuid.UUID, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Generate a new reservation id.
	id := uuid.New()
	if _, ok := c.reservations[id]; ok {
		return uuid.Nil, fmt.Errorf("reservation ID collision")
	}
	r := &reservation{
		controllerSlot: -1,
		partition:      mountConfig.Partition,
	}

	// Determine if this hostPath already had a disk known.
	if slot, ok := c.disksByPath[diskConfig.HostPath]; ok {
		r.controllerSlot = slot // Update our reservation where the disk is.
		d := c.controllerSlots[slot]

		// Verify the caller config is the same.
		if !d.Config().Equals(diskConfig) {
			return uuid.Nil, fmt.Errorf("cannot reserve ref on disk with different config")
		}

		// We at least have a disk, now determine if we have a mount for this
		// partition.
		if _, err := d.ReservePartition(ctx, mountConfig); err != nil {
			return uuid.Nil, fmt.Errorf("reserve partition %d: %w", mountConfig.Partition, err)
		}
	} else {
		// No hostPath was found. Find a slot for the disk.
		nextSlot := -1
		for i, d := range c.controllerSlots {
			if d == nil {
				nextSlot = i
				break
			}
		}
		if nextSlot == -1 {
			return uuid.Nil, fmt.Errorf("no available slots")
		}

		// Create the Disk and Partition Mount in the reserved states.
		controller := uint(nextSlot / numLUNsPerController)
		lun := uint(nextSlot % numLUNsPerController)
		d := disk.NewReserved(controller, lun, diskConfig)
		if _, err := d.ReservePartition(ctx, mountConfig); err != nil {
			return uuid.Nil, fmt.Errorf("reserve partition %d: %w", mountConfig.Partition, err)
		}
		c.controllerSlots[controller*numLUNsPerController+lun] = d
		c.disksByPath[diskConfig.HostPath] = nextSlot
		r.controllerSlot = nextSlot
	}

	// Ensure our reservation is saved for all future operations.
	c.reservations[id] = r
	return id, nil
}

func (c *Controller) MapToGuest(ctx context.Context, reservation uuid.UUID) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if r, ok := c.reservations[reservation]; ok {
		d := c.controllerSlots[r.controllerSlot]
		if err := d.AttachToVM(ctx, c.vm); err != nil {
			return "", fmt.Errorf("attach disk to vm: %w", err)
		}
		guestPath, err := d.MountPartitionToGuest(ctx, r.partition, c.lGuest, c.wGuest)
		if err != nil {
			return "", fmt.Errorf("mount partition %d to guest: %w", r.partition, err)
		}
		return guestPath, nil
	}
	return "", fmt.Errorf("reservation %s not found", reservation)
}

func (c *Controller) UnmapFromGuest(ctx context.Context, reservation uuid.UUID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if r, ok := c.reservations[reservation]; ok {
		d := c.controllerSlots[r.controllerSlot]
		// Ref counted unmount.
		if err := d.UnmountPartitionFromGuest(ctx, r.partition, c.lGuest, c.wGuest); err != nil {
			return fmt.Errorf("unmount partition %d from guest: %w", r.partition, err)
		}
		if err := d.DetachFromVM(ctx, c.vm, c.lGuest); err != nil {
			return fmt.Errorf("detach disk from vm: %w", err)
		}
		if d.State() == disk.DiskStateDetached {
			// If we have no more mounts on this disk, remove the disk from the
			// known disks and free the slot.
			delete(c.disksByPath, d.HostPath())
			c.controllerSlots[r.controllerSlot] = nil
		}
		delete(c.reservations, reservation)
		return nil
	}
	return fmt.Errorf("reservation %s not found", reservation)
}

type reservation struct {
	// This is the index into controllerSlots that holds this disk.
	controllerSlot int
	// This is the index into the disk mounts for this partition.
	partition uint64
}
