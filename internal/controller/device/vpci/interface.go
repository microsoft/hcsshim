//go:build windows

package vpci

import (
	"context"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// Controller manages the lifecycle of vPCI devices assigned to a UVM.
type Controller interface {
	// AddToVM assigns a vPCI device to the VM. If the same device is already
	// assigned, the reference count is incremented.
	AddToVM(ctx context.Context, opts *AddOptions) error

	// RemoveFromVM removes a vPCI device identified by vmBusGUID from the VM.
	// If the device is shared (reference count > 1), the reference count is
	// decremented without actually removing the device.
	RemoveFromVM(ctx context.Context, vmBusGUID string) error
}

// AddOptions holds the configuration required to assign a vPCI device to the VM.
type AddOptions struct {
	// DeviceInstanceID is the host device instance path of the vPCI device.
	DeviceInstanceID string

	// VirtualFunctionIndex is the SR-IOV virtual function index to assign.
	VirtualFunctionIndex uint16

	// VMBusGUID identifies the VirtualPci device (backed by a VMBus channel)
	// inside the UVM.
	VMBusGUID string
}

// vmVPCI manages adding and removing vPCI devices for a Utility VM.
// Implemented by [vmmanager.UtilityVM].
type vmVPCI interface {
	// AddDevice adds a vPCI device identified by `vmBusGUID` to the Utility VM with the provided settings.
	AddDevice(ctx context.Context, vmBusGUID string, settings hcsschema.VirtualPciDevice) error

	// RemoveDevice removes the vPCI device identified by `vmBusGUID` from the Utility VM.
	RemoveDevice(ctx context.Context, vmBusGUID string) error
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

// deviceKey uniquely identifies a host vPCI device by its instance ID and
// virtual function index.
type deviceKey struct {
	deviceInstanceID     string
	virtualFunctionIndex uint16
}

// deviceInfo records one vPCI device's assignment state and reference count.
type deviceInfo struct {
	// key is the immutable host device identifier used to detect duplicate
	// assignment requests.
	key deviceKey

	// vmBusGUID identifies the VirtualPci device (backed by a VMBus channel)
	// inside the UVM.
	vmBusGUID string

	// refCount is the number of active callers sharing this device.
	// Access must be guarded by [Manager.mu].
	refCount uint32

	// invalid indicates the host-side assignment succeeded but the guest-side
	// assignment failed. Access must be guarded by [Manager.mu].
	invalid bool
}
