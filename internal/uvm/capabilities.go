//go:build windows

package uvm

import (
	"github.com/Microsoft/hcsshim/internal/gcs"
)

// SignalProcessSupported returns `true` if the guest supports the capability to
// signal a process.
//
// This support was added RS5+ guests.
func (uvm *UtilityVM) SignalProcessSupported() bool {
	return uvm.guestCaps.IsSignalProcessSupported()
}

func (uvm *UtilityVM) DeleteContainerStateSupported() bool {
	if uvm.gc == nil {
		return false
	}
	return uvm.guestCaps.IsDeleteContainerStateSupported()
}

// Capabilities returns the protocol version and the guest defined capabilities.
// This should only be used for testing.
func (uvm *UtilityVM) Capabilities() (uint32, gcs.GuestDefinedCapabilities) {
	return uvm.protocol, uvm.guestCaps
}
