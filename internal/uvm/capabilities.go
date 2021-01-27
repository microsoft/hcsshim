package uvm

import (
	"github.com/Microsoft/hcsshim/internal/cimfs"
	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
)

// SignalProcessSupported returns `true` if the guest supports the capability to
// signal a process.
//
// This support was added RS5+ guests.
func (uvm *UtilityVM) SignalProcessSupported() bool {
	return uvm.guestCaps.SignalProcessSupported
}

func (uvm *UtilityVM) DeleteContainerStateSupported() bool {
	if uvm.gc == nil {
		return false
	}
	return uvm.guestCaps.DeleteContainerStateSupported
}

// Capabilities returns the protocol version and the guest defined capabilities.
// This should only be used for testing.
func (uvm *UtilityVM) Capabilities() (uint32, schema1.GuestDefinedCapabilities) {
	return uvm.protocol, uvm.guestCaps
}

// MountCimSupported returns true if the uvm allows mounting a cim inside the it (This
// support is available for IRON+ onwards). Returns false otherwise.
func (uvm *UtilityVM) MountCimSupported() bool {
	// Mounting cim inside the uvm is not supported if the uvm is running a windows
	// version < IRON. However, even if the uvm windows version is >= IRON if there
	// are any kinds of devices physically mapped into the uvm then we won't be able
	// to have the layers direct mapped and so we can't mount the layer cim inside the
	// uvm.
	return uvm.buildVersion >= cimfs.MinimumCimFSBuild && !uvm.devicesPhysicallyBacked
}
