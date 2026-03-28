//go:build windows

package network

import (
	"context"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/gcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// SetupOptions holds the configuration required to set up the network for a pod.
type SetupOptions struct {
	// NetworkNamespace is the HCN namespace ID to attach to the guest.
	NetworkNamespace string

	// PolicyBasedRouting controls whether policy-based routing is configured
	// for the endpoints added to the guest. Only relevant for LCOW.
	PolicyBasedRouting bool
}

// capabilitiesProvider is a narrow interface satisfied by guestmanager.Manager.
// It exists so callers pass the guest manager scoped only to Capabilities(),
// avoiding a hard dependency on the full guestmanager.Manager interface here.
type capabilitiesProvider interface {
	Capabilities() gcs.GuestDefinedCapabilities
}

// vmNetworkManager manages adding and removing network adapters for a Utility VM.
// Implemented by vmmanager.UtilityVM.
type vmNetworkManager interface {
	// AddNIC adds a network adapter to the Utility VM. `nicID` should be a string representation of a
	// Windows GUID.
	AddNIC(ctx context.Context, nicID string, settings *hcsschema.NetworkAdapter) error

	// RemoveNIC removes a network adapter from the Utility VM. `nicID` should be a string representation of a
	// Windows GUID.
	RemoveNIC(ctx context.Context, nicID string, settings *hcsschema.NetworkAdapter) error
}

// linuxGuestNetworkManager exposes linux guest network operations.
// Implemented by guestmanager.Guest.
type linuxGuestNetworkManager interface {
	// AddLCOWNetworkInterface adds a network interface to the LCOW guest.
	AddLCOWNetworkInterface(ctx context.Context, settings *guestresource.LCOWNetworkAdapter) error
	// RemoveLCOWNetworkInterface removes a network interface from the LCOW guest.
	RemoveLCOWNetworkInterface(ctx context.Context, settings *guestresource.LCOWNetworkAdapter) error
}

// windowsGuestNetworkManager exposes windows guest network operations.
// Implemented by guestmanager.Guest.
type windowsGuestNetworkManager interface {
	// AddNetworkNamespace adds a network namespace to the WCOW guest.
	AddNetworkNamespace(ctx context.Context, settings *hcn.HostComputeNamespace) error
	// RemoveNetworkNamespace removes a network namespace from the WCOW guest.
	RemoveNetworkNamespace(ctx context.Context, settings *hcn.HostComputeNamespace) error
	// AddNetworkInterface adds a network interface to the WCOW guest.
	AddNetworkInterface(ctx context.Context, adapterID string, requestType guestrequest.RequestType, settings *hcn.HostComputeEndpoint) error
	// RemoveNetworkInterface removes a network interface from the WCOW guest.
	RemoveNetworkInterface(ctx context.Context, adapterID string, requestType guestrequest.RequestType, settings *hcn.HostComputeEndpoint) error
}
