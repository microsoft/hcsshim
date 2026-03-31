//go:build windows

package vpci

import (
	"context"
	"fmt"
	"sync"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/sirupsen/logrus"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
)

// Controller manages vPCI device assignments for a Utility VM.
type Controller struct {
	mu sync.Mutex

	// devices tracks currently assigned vPCI devices, keyed by VMBus GUID.
	// Guarded by mu.
	devices map[guid.GUID]*deviceInfo

	// deviceToGUID maps a [Device] to its VMBus GUID for duplicate detection
	// during [Controller.Reserve]. Guarded by mu.
	deviceToGUID map[Device]guid.GUID

	// vmVPCI performs host-side vPCI device add/remove on the VM.
	vmVPCI vmVPCI

	// linuxGuestVPCI performs guest-side vPCI device setup for LCOW.
	linuxGuestVPCI linuxGuestVPCI
}

// New creates a ready-to-use [Controller].
func New(
	vmVPCI vmVPCI,
	linuxGuestVPCI linuxGuestVPCI,
) *Controller {
	return &Controller{
		vmVPCI:         vmVPCI,
		linuxGuestVPCI: linuxGuestVPCI,
		devices:        make(map[guid.GUID]*deviceInfo),
		deviceToGUID:   make(map[Device]guid.GUID),
	}
}

// Reserve generates a unique VMBus GUID for the given vPCI device and records
// the reservation. The returned GUID can later be passed to [Controller.AddToVM]
// to actually assign the device to the VM.
//
// If the same device (identified by DeviceInstanceID and VirtualFunctionIndex) has
// already been reserved, the existing GUID is returned.
//
// Each Virtual Function is assigned as an independent guest device with its own
// VMBus GUID. Multiple Virtual Functions on the same physical device are treated
// as separate devices.
func (c *Controller) Reserve(ctx context.Context, device Device) (guid.GUID, error) {
	ctx, _ = log.WithContext(ctx, logrus.WithFields(logrus.Fields{
		logfields.DeviceID: device.DeviceInstanceID,
		logfields.VFIndex:  device.VirtualFunctionIndex,
	}))

	c.mu.Lock()
	defer c.mu.Unlock()

	// If this device is already reserved, return the existing GUID.
	if existingGUID, ok := c.deviceToGUID[device]; ok {
		log.G(ctx).WithField(logfields.VMBusGUID, existingGUID).Debug("vPCI device already reserved, reusing existing GUID")
		return existingGUID, nil
	}

	// Generate a new VMBus GUID for this device.
	vmBusGUID, err := guid.NewV4()
	if err != nil {
		return guid.GUID{}, fmt.Errorf("generate vmbus guid for device %s: %w", device.DeviceInstanceID, err)
	}

	c.devices[vmBusGUID] = &deviceInfo{
		device:    device,
		vmBusGUID: vmBusGUID,
	}
	c.deviceToGUID[device] = vmBusGUID

	log.G(ctx).WithField(logfields.VMBusGUID, vmBusGUID).Debug("reserved vPCI device with new VMBus GUID")
	return vmBusGUID, nil
}

// AddToVM assigns a previously reserved vPCI device to the VM.
// The vmBusGUID must have been obtained from a prior call to [Controller.Reserve].
// If the device is already assigned to the VM, the existing assignment is reused.
func (c *Controller) AddToVM(ctx context.Context, vmBusGUID guid.GUID) error {
	// Set vmBusGUID in logging context.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.VMBusGUID, vmBusGUID))

	c.mu.Lock()
	defer c.mu.Unlock()

	dev, ok := c.devices[vmBusGUID]
	if !ok {
		return fmt.Errorf("no reservation found for vmBusGUID %s; call Reserve first", vmBusGUID)
	}

	// If a previous assignment left the device in an invalid state,
	// reject new callers until the existing assignment is cleaned up.
	if dev.invalid {
		return fmt.Errorf("vpci device with vmBusGUID %s is in an invalid state", vmBusGUID)
	}

	ctx, _ = log.WithContext(ctx, logrus.WithFields(logrus.Fields{
		logfields.DeviceID: dev.device.DeviceInstanceID,
		logfields.VFIndex:  dev.device.VirtualFunctionIndex,
	}))

	// If the device is already assigned to the VM (host-side call was already made),
	// just bump the reference count and return.
	if dev.refCount > 0 {
		dev.refCount++

		log.G(ctx).Debug("vPCI device already assigned, reusing existing assignment")

		return nil
	}

	// Device not yet attached to VM.
	log.G(ctx).Debug("assigning vPCI device to VM")

	// NUMA affinity is always propagated for assigned devices.
	// This feature is available on WS2025 and later.
	// Since the V2 shims only support WS2025 and later, this is set as true.
	propagateAffinity := true

	settings := hcsschema.VirtualPciDevice{
		Functions: []hcsschema.VirtualPciFunction{
			{
				DeviceInstancePath: dev.device.DeviceInstanceID,
				VirtualFunction:    dev.device.VirtualFunctionIndex,
			},
		},
		PropagateNumaAffinity: &propagateAffinity,
	}

	guidStr := vmBusGUID.String()

	// Host-side: add the vPCI device to the VM.
	if err := c.vmVPCI.AddDevice(ctx, guidStr, settings); err != nil {
		return fmt.Errorf("add vpci device %s to vm: %w", dev.device.DeviceInstanceID, err)
	}

	// Update the ref count to indicate the device is now assigned to the VM.
	dev.refCount++

	// Guest-side: device attach notification.
	if err := c.waitGuestDeviceReady(ctx, guidStr); err != nil {
		// Mark the device as invalid so the caller can call RemoveFromVM
		// to clean up the host-side assignment.
		dev.invalid = true
		log.G(ctx).WithError(err).Error("guest-side vpci device setup failed, device marked invalid")
		return fmt.Errorf("add guest vpci device with vmBusGUID %s to vm: %w", vmBusGUID, err)
	}

	log.G(ctx).Info("vPCI device assigned to VM")

	return nil
}

// RemoveFromVM removes a vPCI device from the VM.
// If the device is shared (reference count > 1), the reference count is
// decremented without actually removing the device from the VM.
func (c *Controller) RemoveFromVM(ctx context.Context, vmBusGUID guid.GUID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.VMBusGUID, vmBusGUID))

	dev, ok := c.devices[vmBusGUID]
	if !ok {
		return fmt.Errorf("no vpci device with vmBusGUID %s is assigned to the vm", vmBusGUID)
	}

	// Device was reserved but never added to the VM. Just clean up the reservation.
	if dev.refCount == 0 {
		log.G(ctx).Debug("vPCI device was reserved but never assigned, cleaning up reservation")

		delete(c.devices, vmBusGUID)
		delete(c.deviceToGUID, dev.device)

		return nil
	}

	// Decrement the ref count for the device.
	dev.refCount--
	if dev.refCount > 0 {
		log.G(ctx).WithField("refCount", dev.refCount).Debug("vPCI device still in use, decremented ref count")
		return nil
	}

	// Last reference dropped (refCount == 0). Remove the device from the VM.
	// This also covers devices marked invalid during AddToVM — the host-side
	// assignment still needs to be cleaned up.

	log.G(ctx).Debug("removing vPCI device from VM")

	// Host-side: remove the vPCI device from the VM.
	if err := c.vmVPCI.RemoveDevice(ctx, vmBusGUID.String()); err != nil {
		// Restore the ref count since the removal failed.
		dev.refCount++
		return fmt.Errorf("remove vpci device %s from vm: %w", vmBusGUID, err)
	}

	delete(c.devices, vmBusGUID)
	delete(c.deviceToGUID, dev.device)

	log.G(ctx).Info("vPCI device removed from VM")

	return nil
}
