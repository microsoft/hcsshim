package uvm

import "github.com/Microsoft/hcsshim/internal/schema1"

// SignalProcessSupported returns `true` if the guest supports the capability to
// signal a process.
//
// This support was added RS5+ guests.
func (uvm *UtilityVM) SignalProcessSupported() bool {
	return uvm.guestCaps.SignalProcessSupported
}

// Capabilities returns the protocol version and the guest defined capabilities.
// This should only be used for testing.
func (uvm *UtilityVM) Capabilities() (uint32, schema1.GuestDefinedCapabilities) {
	return uvm.protocol, uvm.guestCaps
}
