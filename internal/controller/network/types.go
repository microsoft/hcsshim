//go:build windows && (lcow || wcow)

package network

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/gcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// Options holds the configuration for the controller which would be required
// to set up the network for a pod.
type Options struct {
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
