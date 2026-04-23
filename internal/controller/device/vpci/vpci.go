//go:build windows && (lcow || wcow)

package vpci

import (
	"context"
	"fmt"
	"sync"

	"github.com/Microsoft/go-winio/pkg/guid"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/sirupsen/logrus"

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

	// guestVPCI performs guest-side vPCI device setup.
	guestVPCI guestVPCI
}

// New creates a ready-to-use [Controller].
func New(
	vmVPCI vmVPCI,
	guestVPCI guestVPCI,
) *Controller {
	return &Controller{
		vmVPCI:       vmVPCI,
		guestVPCI:    guestVPCI,
		devices:      make(map[guid.GUID]*deviceInfo),
		deviceToGUID: make(map[Device]guid.GUID),
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
		state:     StateReserved,
	}
	c.deviceToGUID[device] = vmBusGUID

	log.G(ctx).WithField(logfields.VMBusGUID, vmBusGUID).Debug("reserved vPCI device with new VMBus GUID")
	return vmBusGUID, nil
}

// AddToVM assigns a previously reserved vPCI device to the VM.
// The vmBusGUID must have been obtained from a prior call to [Controller.Reserve].
// If the device is already ready for use in the VM, the reference count is incremented.
//
// On failure the caller should call [Controller.RemoveFromVM] to clean up.
func (c *Controller) AddToVM(ctx context.Context, vmBusGUID guid.GUID) error {
	// Set vmBusGUID in logging context.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.VMBusGUID, vmBusGUID))

	c.mu.Lock()
	defer c.mu.Unlock()

	dev, ok := c.devices[vmBusGUID]
	if !ok {
		return fmt.Errorf("no reservation found for vmBusGUID %s; call Reserve first", vmBusGUID)
	}

	ctx, _ = log.WithContext(ctx, logrus.WithFields(logrus.Fields{
		logfields.DeviceID: dev.device.DeviceInstanceID,
		logfields.VFIndex:  dev.device.VirtualFunctionIndex,
	}))

	switch dev.state {
	case StateReady:
		// Device is already fully assigned and guest-ready; just bump the ref count.
		dev.refCount++
		log.G(ctx).Debug("vPCI device already ready, reusing existing assignment")

	case StateReserved:
		// Device not yet attached to VM — perform the host-side add.
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

		// Host-side: add the vPCI device to the VM.
		if err := c.vmVPCI.AddDevice(ctx, vmBusGUID, settings); err != nil {
			// Set state to removed on failure.
			// The caller can call RemoveFromVM to clean up the reservation.
			dev.state = StateRemoved
			return fmt.Errorf("add vpci device %s to vm: %w", dev.device.DeviceInstanceID, err)
		}

		// Host-side succeeded; mark as assigned (transient state) before
		// waiting for the guest.
		dev.state = StateAssigned

		// Guest-side: wait for the device to be ready inside the guest.
		if err := c.waitGuestDeviceReady(ctx, vmBusGUID); err != nil {
			// Host assignment is in place but guest is not ready.
			// Mark StateAssignedInvalid so the caller can call RemoveFromVM
			// to clean up the host-side assignment.
			dev.state = StateAssignedInvalid
			return fmt.Errorf("wait for guest vpci device with vmBusGUID %s to become ready: %w", vmBusGUID, err)
		}

		// Both host and guest succeeded; device is fully ready.
		dev.refCount++
		dev.state = StateReady

		log.G(ctx).Info("vPCI device assigned to VM")

	case StateAssignedInvalid:
		// The device add failed in a previous attempt after the host-side assignment
		// succeeded. Call RemoveFromVM to clean up the host-side assignment before retrying.
		return fmt.Errorf("vpci device with vmBusGUID %s is in an invalid state; call RemoveFromVM first", vmBusGUID)

	case StateRemoved:
		// The device failed to be added to the VM and hence was moved to state removed.
		return fmt.Errorf("vpci device with vmBusGUID %s was removed due to a prior failure; call RemoveFromVM first", vmBusGUID)

	default:
		// StateAssigned should never be observed by callers (it is a transient
		// within-call state).
		return fmt.Errorf("vpci device with vmBusGUID %s is in an unexpected state %s", vmBusGUID, dev.state)
	}

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

	switch dev.state {
	case StateReserved, StateRemoved:
		// Device was reserved but never assigned to the VM (or never assigned).
		// No host-side state to clean up — just drop the tracking entry.
		log.G(ctx).WithField("state", dev.state).Debug("vPCI device has no host-side assignment, cleaning up reservation")
		c.untrack(vmBusGUID, dev)
		return nil

	case StateReady:
		// Decrement ref count; only remove from the host when the last reference is dropped.
		if dev.refCount > 1 {
			dev.refCount--
			log.G(ctx).WithField("refCount", dev.refCount).Debug("vPCI device still in use, decremented ref count")
			return nil
		}

		// Last reference — fall through to host-side remove below.
		dev.refCount = 0

	case StateAssignedInvalid:
		// Host-side assignment exists but device is in an inconsistent state.
		// Proceed directly to host-side remove (refCount is always 0 here).

	default:
		// StateAssigned is a transient within-call state and should not be seen here.
		return fmt.Errorf("vpci device with vmBusGUID %s is in an unexpected state %s", vmBusGUID, dev.state)
	}

	log.G(ctx).Debug("removing vPCI device from VM")

	// Host-side: remove the vPCI device from the VM.
	if err := c.vmVPCI.RemoveDevice(ctx, vmBusGUID); err != nil {
		// The host-side remove failed; the device is still partially assigned.
		// Mark it StateAssignedInvalid so that callers can retry via RemoveFromVM.
		dev.state = StateAssignedInvalid
		return fmt.Errorf("remove vpci device %s from vm: %w", vmBusGUID, err)
	}

	c.untrack(vmBusGUID, dev)
	log.G(ctx).Info("vPCI device removed from VM")
	return nil
}

// untrack removes a device from the controller's tracking maps and sets its
// state to [StateRemoved] as a safety marker.
// Must be called with c.mu held.
func (c *Controller) untrack(vmBusGUID guid.GUID, dev *deviceInfo) {
	dev.state = StateRemoved
	delete(c.devices, vmBusGUID)
	delete(c.deviceToGUID, dev.device)
}
