//go:build windows && lcow

package network

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/hcn"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// guestNetwork exposes linux guest network operations.
// Implemented by guestmanager.Guest.
type guestNetwork interface {
	// AddLCOWNetworkInterface adds a network interface to the LCOW guest.
	AddLCOWNetworkInterface(ctx context.Context, settings *guestresource.LCOWNetworkAdapter) error
	// RemoveLCOWNetworkInterface removes a network interface from the LCOW guest.
	RemoveLCOWNetworkInterface(ctx context.Context, settings *guestresource.LCOWNetworkAdapter) error
}

// addNetNSInsideGuest maps a host network namespace into the guest as a managed Guest Network Namespace.
// This is a no-op for LCOW as the network namespace is created via pause container
// and the adapters are added dynamically.
func (c *Controller) addNetNSInsideGuest(_ context.Context, _ *hcn.HostComputeNamespace) error {
	return nil
}

// removeNetNSInsideGuest is a no-op for LCOW; the guest-managed namespace
// is torn down automatically when pause container exits.
func (c *Controller) removeNetNSInsideGuest(_ context.Context, _ string) error {
	return nil
}

// addEndpointToGuestNamespace hot-adds an HCN endpoint to the UVM and,
// configures it inside the LCOW guest.
func (c *Controller) addEndpointToGuestNamespace(ctx context.Context, nicID string, endpoint *hcn.HostComputeEndpoint, isPolicyBasedRoutingSupported bool) error {
	log.G(ctx).Info("adding endpoint to guest namespace")

	// 1. Host-side hot-add.
	if err := c.vmNetwork.AddNIC(ctx, nicID, &hcsschema.NetworkAdapter{
		EndpointId: endpoint.Id,
		MacAddress: endpoint.MacAddress,
	}); err != nil {
		return fmt.Errorf("add NIC %s to host (endpoint %s): %w", nicID, endpoint.Id, err)
	}

	log.G(ctx).Debug("added NIC to host")

	// Track early so Teardown cleans up even if the guest Add call fails.
	c.vmEndpoints[nicID] = endpoint

	// 2. Guest-side add.
	if c.isNamespaceSupportedByGuest {
		lcowAdapter, err := guestresource.BuildLCOWNetworkAdapter(nicID, endpoint, isPolicyBasedRoutingSupported)
		if err != nil {
			return fmt.Errorf("build LCOW network adapter for endpoint %s: %w", endpoint.Id, err)
		}

		log.G(ctx).Tracef("built LCOW network adapter: %+v", lcowAdapter)

		if err := c.guestNetwork.AddLCOWNetworkInterface(ctx, lcowAdapter); err != nil {
			return fmt.Errorf("add NIC %s to guest (endpoint %s): %w", nicID, endpoint.Id, err)
		}

		log.G(ctx).Debug("nic configured in guest")
	}

	return nil
}

// removeEndpointFromGuestNamespace removes an endpoint from the LCOW guest
// and then hot-removes the NIC from the host.
func (c *Controller) removeEndpointFromGuestNamespace(ctx context.Context, nicID string, endpoint *hcn.HostComputeEndpoint) error {
	log.G(ctx).Info("removing endpoint from guest namespace")

	if c.isNamespaceSupportedByGuest {
		// 1. LCOW guest-side removal.
		if err := c.guestNetwork.RemoveLCOWNetworkInterface(ctx, &guestresource.LCOWNetworkAdapter{
			NamespaceID: c.namespaceID,
			ID:          nicID,
		}); err != nil {
			return fmt.Errorf("remove NIC %s from guest: %w", nicID, err)
		}

		log.G(ctx).Debug("removed NIC from guest")
	}

	// 2. Host-side removal.
	if err := c.vmNetwork.RemoveNIC(ctx, nicID, &hcsschema.NetworkAdapter{
		EndpointId: endpoint.Id,
		MacAddress: endpoint.MacAddress,
	}); err != nil {
		return fmt.Errorf("remove NIC %s from host (endpoint %s): %w", nicID, endpoint.Id, err)
	}

	log.G(ctx).Debug("removed NIC from host")

	return nil
}
