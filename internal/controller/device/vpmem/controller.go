//go:build windows

package vpmem

import (
	"context"
	"fmt"
	"sync"

	"github.com/Microsoft/hcsshim/internal/controller/device/vpmem/device"
	"github.com/Microsoft/hcsshim/internal/controller/device/vpmem/mount"

	"github.com/google/uuid"
)

type VMVPMemOps interface {
	device.VMVPMemAdder
	device.VMVPMemRemover
}

type LinuxGuestVPMemOps interface {
	mount.LinuxGuestVPMemMounter
	mount.LinuxGuestVPMemUnmounter
}

// The controller manages all VPMem attached devices and guest mounted
// directories.
//
// It is required that all callers:
//
// 1. Obtain a reservation using Reserve().
//
// 2. Use the reservation to Mount() to ensure resource availability.
//
// 3. Call Unmount() to release the reservation and all resources.
//
// If Mount() fails, the caller must call Unmount() to release the reservation
// and all resources.
//
// If Unmount() fails, the caller must call Unmount() again until it succeeds to
// release the reservation and all resources.
type Controller struct {
	vm    VMVPMemOps
	guest LinuxGuestVPMemOps

	mu sync.Mutex

	// Every call to Reserve gets a unique reservation ID which holds the slot
	// index for the device.
	reservations map[uuid.UUID]*reservation

	// For fast lookup we keep a hostPath to slot mapping for all allocated
	// devices.
	devicesByPath map[string]uint32

	// Tracks all allocated and unallocated available VPMem device slots.
	slots []*device.Device
}

func New(maxDevices uint32, vm VMVPMemOps, guest LinuxGuestVPMemOps) *Controller {
	return &Controller{
		vm:            vm,
		guest:         guest,
		mu:            sync.Mutex{},
		reservations:  make(map[uuid.UUID]*reservation),
		devicesByPath: make(map[string]uint32),
		slots:         make([]*device.Device, maxDevices),
	}
}

// Reserve creates a referenced counted mapping entry for a VPMem attachment
// based on the device host path.
//
// If an error is returned from this function, it is guaranteed that no
// reservation mapping was made and no Unmount() call is necessary to clean up.
func (c *Controller) Reserve(ctx context.Context, deviceConfig device.DeviceConfig, mountConfig mount.MountConfig) (uuid.UUID, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Generate a new reservation id.
	id := uuid.New()
	if _, ok := c.reservations[id]; ok {
		return uuid.Nil, fmt.Errorf("reservation ID collision")
	}
	r := &reservation{}

	// Determine if this hostPath already had a device known.
	if slot, ok := c.devicesByPath[deviceConfig.HostPath]; ok {
		r.slot = slot
		d := c.slots[slot]

		// Verify the caller config is the same.
		if !d.Config().Equals(deviceConfig) {
			return uuid.Nil, fmt.Errorf("cannot reserve ref on device with different config")
		}

		if _, err := d.ReserveMount(ctx, mountConfig); err != nil {
			return uuid.Nil, fmt.Errorf("reserve mount: %w", err)
		}
	} else {
		// No hostPath was found. Find a slot for the device.
		nextSlot := uint32(0)
		found := false
		for i := uint32(0); i < uint32(len(c.slots)); i++ {
			if c.slots[i] == nil {
				nextSlot = i
				found = true
				break
			}
		}
		if !found {
			return uuid.Nil, fmt.Errorf("no available slots")
		}

		// Create the Device and Mount in the reserved states.
		d := device.NewReserved(nextSlot, deviceConfig)
		if _, err := d.ReserveMount(ctx, mountConfig); err != nil {
			return uuid.Nil, fmt.Errorf("reserve mount: %w", err)
		}
		c.slots[nextSlot] = d
		c.devicesByPath[deviceConfig.HostPath] = nextSlot
		r.slot = nextSlot
	}

	// Ensure our reservation is saved for all future operations.
	c.reservations[id] = r
	return id, nil
}

func (c *Controller) Mount(ctx context.Context, reservation uuid.UUID) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if r, ok := c.reservations[reservation]; ok {
		d := c.slots[r.slot]
		if err := d.AttachToVM(ctx, c.vm); err != nil {
			return "", fmt.Errorf("attach device to vm: %w", err)
		}
		guestPath, err := d.MountToGuest(ctx, c.guest)
		if err != nil {
			return "", fmt.Errorf("mount to guest: %w", err)
		}
		return guestPath, nil
	}
	return "", fmt.Errorf("reservation %s not found", reservation)
}

func (c *Controller) Unmount(ctx context.Context, reservation uuid.UUID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if r, ok := c.reservations[reservation]; ok {
		d := c.slots[r.slot]
		if err := d.UnmountFromGuest(ctx, c.guest); err != nil {
			return fmt.Errorf("unmount from guest: %w", err)
		}
		if err := d.DetachFromVM(ctx, c.vm); err != nil {
			return fmt.Errorf("detach device from vm: %w", err)
		}
		if d.State() == device.DeviceStateDetached {
			delete(c.devicesByPath, d.HostPath())
			c.slots[r.slot] = nil
		}
		delete(c.reservations, reservation)
		return nil
	}
	return fmt.Errorf("reservation %s not found", reservation)
}

type reservation struct {
	slot uint32
}
