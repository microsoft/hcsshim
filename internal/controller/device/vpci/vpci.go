//go:build windows

package vpci

import (
	"context"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
)

// Manager is the concrete implementation of [Controller].
type Manager struct {
	mu sync.Mutex

	// devices tracks currently assigned vPCI devices, keyed by VMBus GUID.
	// Guarded by mu.
	devices map[string]*deviceInfo

	// keyToGUID maps a [deviceKey] to its VMBus GUID for duplicate detection
	// during [Manager.AddToVM]. Guarded by mu.
	keyToGUID map[deviceKey]string

	// vmVPCI performs host-side vPCI device add/remove on the VM.
	vmVPCI vmVPCI

	// linuxGuestVPCI performs guest-side vPCI device setup for LCOW.
	linuxGuestVPCI linuxGuestVPCI
}

var _ Controller = (*Manager)(nil)

// New creates a ready-to-use [Manager].
func New(
	vmVPCI vmVPCI,
	linuxGuestVPCI linuxGuestVPCI,
) *Manager {
	return &Manager{
		vmVPCI:         vmVPCI,
		linuxGuestVPCI: linuxGuestVPCI,
		devices:        make(map[string]*deviceInfo),
		keyToGUID:      make(map[deviceKey]string),
	}
}

// AddToVM assigns a vPCI device to the VM.
// If the same device is already assigned, the existing assignment is reused.
func (m *Manager) AddToVM(ctx context.Context, opts *AddOptions) (err error) {
	if opts.VMBusGUID == "" {
		return fmt.Errorf("vmbus guid is required in add options")
	}

	key := deviceKey{
		deviceInstanceID:     opts.DeviceInstanceID,
		virtualFunctionIndex: opts.VirtualFunctionIndex,
	}

	// Set vmBusGUID in logging context.
	ctx, _ = log.WithContext(ctx, logrus.WithField("vmBusGUID", opts.VMBusGUID))

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for an existing assignment with the same device key.
	if existingGUID, ok := m.keyToGUID[key]; ok {
		dev := m.devices[existingGUID]

		// If a previous assignment left the device in an invalid state
		// reject new callers until the existing assignment is cleaned up.
		if dev.invalid {
			return fmt.Errorf("vpci device with vmBusGUID %s is in an invalid state", existingGUID)
		}

		// Increase the refCount and return the existing device.
		dev.refCount++

		log.G(ctx).WithFields(logrus.Fields{
			"deviceInstanceID":     key.deviceInstanceID,
			"virtualFunctionIndex": key.virtualFunctionIndex,
			"refCount":             dev.refCount,
		}).Debug("vPCI device already assigned, reusing existing assignment")

		return nil
	}

	// Device not attached to VM.
	// Build the VirtualPciDevice settings for HCS call.

	log.G(ctx).WithFields(logrus.Fields{
		"deviceInstanceID":     key.deviceInstanceID,
		"virtualFunctionIndex": key.virtualFunctionIndex,
	}).Debug("assigning vPCI device to VM")

	// NUMA affinity is always propagated for assigned devices.
	// This feature is available on WS2025 and later.
	// Since the V2 shims only support WS2025 and later, this is set as true.
	propagateAffinity := true

	settings := hcsschema.VirtualPciDevice{
		Functions: []hcsschema.VirtualPciFunction{
			{
				DeviceInstancePath: opts.DeviceInstanceID,
				VirtualFunction:    opts.VirtualFunctionIndex,
			},
		},
		PropagateNumaAffinity: &propagateAffinity,
	}

	// Host-side: add the vPCI device to the VM.
	if err := m.vmVPCI.AddDevice(ctx, opts.VMBusGUID, settings); err != nil {
		return fmt.Errorf("add vpci device %s to vm: %w", opts.DeviceInstanceID, err)
	}

	// Track early so RemoveFromVM can clean up even if the guest-side call fails.
	dev := &deviceInfo{
		key:       key,
		vmBusGUID: opts.VMBusGUID,
		refCount:  1,
	}
	m.devices[opts.VMBusGUID] = dev
	m.keyToGUID[key] = opts.VMBusGUID

	// Guest-side: device attach notification.
	if err := m.addGuestVPCIDevice(ctx, opts.VMBusGUID); err != nil {
		// Mark the device as invalid so the caller can call RemoveFromVM
		// to clean up the host-side assignment.
		dev.invalid = true
		log.G(ctx).WithError(err).Error("guest-side vpci device setup failed, device marked invalid")
		return fmt.Errorf("add guest vpci device with vmBusGUID %s to vm: %w", opts.VMBusGUID, err)
	}

	log.G(ctx).Info("vPCI device assigned to VM")

	return nil
}

// RemoveFromVM removes a vPCI device from the VM.
// If the device is shared (reference count > 1), the reference count is
// decremented without actually removing the device from the VM.
func (m *Manager) RemoveFromVM(ctx context.Context, vmBusGUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx, _ = log.WithContext(ctx, logrus.WithField("vmBusGUID", vmBusGUID))

	dev, ok := m.devices[vmBusGUID]
	if !ok {
		return fmt.Errorf("no vpci device with vmBusGUID %s is assigned to the vm", vmBusGUID)
	}

	dev.refCount--
	if dev.refCount > 0 {
		log.G(ctx).WithField("refCount", dev.refCount).Debug("vPCI device still in use, decremented ref count")
		return nil
	}

	// This path is reached when the device is no longer shared (refCount == 0) or
	// had transitioned into an invalid state during AddToVM call.

	log.G(ctx).Debug("removing vPCI device from VM")

	// Host-side: remove the vPCI device from the VM.
	if err := m.vmVPCI.RemoveDevice(ctx, vmBusGUID); err != nil {
		// Restore the ref count since the removal failed.
		dev.refCount++
		return fmt.Errorf("remove vpci device %s from vm: %w", vmBusGUID, err)
	}

	delete(m.devices, vmBusGUID)
	delete(m.keyToGUID, dev.key)

	log.G(ctx).Info("vPCI device removed from VM")

	return nil
}
