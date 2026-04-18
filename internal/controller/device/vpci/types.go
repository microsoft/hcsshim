//go:build windows

package vpci

import (
	"context"

	"github.com/Microsoft/go-winio/pkg/guid"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// Device holds the configuration required to assign a vPCI device to the VM.
type Device struct {
	// DeviceInstanceID is the host device instance path of the vPCI device.
	DeviceInstanceID string

	// VirtualFunctionIndex is the SR-IOV virtual function index to assign.
	VirtualFunctionIndex uint16
}

// vmVPCI manages adding and removing vPCI devices for a Utility VM.
// Implemented by [vmmanager.UtilityVM].
type vmVPCI interface {
	// AddDevice adds a vPCI device identified by `vmBusGUID` to the Utility VM with the provided settings.
	AddDevice(ctx context.Context, vmbusGUID guid.GUID, settings hcsschema.VirtualPciDevice) error

	// RemoveDevice removes the vPCI device identified by `vmBusGUID` from the Utility VM.
	RemoveDevice(ctx context.Context, vmbusGUID guid.GUID) error
}

// linuxGuestVPCI exposes vPCI device operations in the LCOW guest.
// Implemented by [guestmanager.Guest].
type linuxGuestVPCI interface {
	// AddVPCIDevice adds a vPCI device to the guest.
	AddVPCIDevice(ctx context.Context, settings guestresource.LCOWMappedVPCIDevice) error
}

// ==============================================================================
// INTERNAL DATA STRUCTURES
// ==============================================================================

// deviceInfo records one vPCI device's assignment state and reference count.
type deviceInfo struct {
	// device is the immutable host device identifier used to detect duplicate
	// assignment requests.
	device Device

	// vmBusGUID identifies the vPCI device (backed by a VMBus channel)
	// inside the UVM.
	vmBusGUID guid.GUID

	// state is the current lifecycle state of this device assignment.
	// Access must be guarded by [Controller.mu].
	state State

	// refCount is the number of active callers sharing this device.
	// Access must be guarded by [Controller.mu].
	refCount uint32
}
