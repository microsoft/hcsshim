//go:build windows && wcow

package network

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/hcn"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"

	"github.com/sirupsen/logrus"
)

// guestNetwork exposes windows guest network operations.
// Implemented by guestmanager.Guest.
type guestNetwork interface {
	// AddNetworkNamespace adds a network namespace to the WCOW guest.
	AddNetworkNamespace(ctx context.Context, settings *hcn.HostComputeNamespace) error
	// RemoveNetworkNamespace removes a network namespace from the WCOW guest.
	RemoveNetworkNamespace(ctx context.Context, settings *hcn.HostComputeNamespace) error
	// AddNetworkInterface adds a network interface to the WCOW guest.
	AddNetworkInterface(ctx context.Context, adapterID string, requestType guestrequest.RequestType, settings *hcn.HostComputeEndpoint) error
	// RemoveNetworkInterface removes a network interface from the WCOW guest.
	RemoveNetworkInterface(ctx context.Context, adapterID string, requestType guestrequest.RequestType, settings *hcn.HostComputeEndpoint) error
}

// addNetNSInsideGuest maps a host network namespace into the guest as a managed Guest Network Namespace.
// For WCOWs, this method sends a request to GCS for adding the namespace.
// GCS forwards the request to GNS which coordinates with HNS to add the namespace to the guest.
func (c *Controller) addNetNSInsideGuest(ctx context.Context, hcnNamespace *hcn.HostComputeNamespace) error {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Namespace, hcnNamespace.Id))

	if c.isNamespaceSupportedByGuest {
		log.G(ctx).Info("adding network namespace to guest")

		if err := c.guestNetwork.AddNetworkNamespace(ctx, hcnNamespace); err != nil {
			return fmt.Errorf("add network namespace %s to guest: %w", hcnNamespace.Id, err)
		}
	}

	return nil
}

// removeNetNSInsideGuest removes the HCN namespace from the WCOW guest via GCS/GNS.
func (c *Controller) removeNetNSInsideGuest(ctx context.Context, namespaceID string) error {
	if c.isNamespaceSupportedByGuest {
		log.G(ctx).Info("removing network namespace from guest")

		hcnNamespace, err := hcn.GetNamespaceByID(namespaceID)
		if err != nil {
			return fmt.Errorf("get network namespace %s: %w", namespaceID, err)
		}

		if err := c.guestNetwork.RemoveNetworkNamespace(ctx, hcnNamespace); err != nil {
			return fmt.Errorf("remove network namespace %s from guest: %w", namespaceID, err)
		}
	}

	return nil
}

// addEndpointToGuestNamespace wires an HCN endpoint into the WCOW guest in three steps:
// pre-add (guest notification), host-side hot-add, and guest-side finalisation.
func (c *Controller) addEndpointToGuestNamespace(ctx context.Context, nicID string, endpoint *hcn.HostComputeEndpoint, _ bool) error {
	log.G(ctx).Info("adding network endpoint to guest namespace")

	// 1. Guest pre-add: informs WCOW guest that a NIC is about to arrive.
	if err := c.guestNetwork.AddNetworkInterface(
		ctx,
		nicID,
		guestrequest.RequestTypePreAdd,
		endpoint,
	); err != nil {
		return fmt.Errorf("pre-add NIC %s to guest (endpoint %s): %w", nicID, endpoint.Id, err)
	}

	log.G(ctx).Info("pre-added network endpoint to guest namespace")

	// 2. Host-side hot-add.
	if err := c.vmNetwork.AddNIC(ctx, nicID, &hcsschema.NetworkAdapter{
		EndpointId: endpoint.Id,
		MacAddress: endpoint.MacAddress,
	}); err != nil {
		return fmt.Errorf("add NIC %s to host (endpoint %s): %w", nicID, endpoint.Id, err)
	}

	log.G(ctx).Info("hot-added network endpoint to host")

	// Track early so Teardown cleans up even if the guest Add call fails.
	c.vmEndpoints[nicID] = endpoint

	// 3. Guest add: finalise the NIC in the WCOW guest.
	if err := c.guestNetwork.AddNetworkInterface(
		ctx,
		nicID,
		guestrequest.RequestTypeAdd,
		nil, // No additional info is needed for the Add call.
	); err != nil {
		return fmt.Errorf("add NIC %s to guest (endpoint %s): %w", nicID, endpoint.Id, err)
	}

	log.G(ctx).Info("configured network endpoint in guest namespace")

	return nil
}

// removeEndpointFromGuestNamespace removes an endpoint from the WCOW guest and then
// hot-removes the NIC from the host.
func (c *Controller) removeEndpointFromGuestNamespace(ctx context.Context, nicID string, endpoint *hcn.HostComputeEndpoint) error {
	log.G(ctx).Info("removing network endpoint from guest namespace")

	// 1. Guest-side removal.
	if err := c.guestNetwork.RemoveNetworkInterface(
		ctx,
		nicID,
		guestrequest.RequestTypeRemove,
		nil,
	); err != nil {
		return fmt.Errorf("remove NIC %s from guest (endpoint %s): %w", nicID, endpoint.Id, err)
	}

	log.G(ctx).Info("removed network endpoint from guest namespace")

	// 2. Host-side removal.
	if err := c.vmNetwork.RemoveNIC(ctx, nicID, &hcsschema.NetworkAdapter{
		EndpointId: endpoint.Id,
		MacAddress: endpoint.MacAddress,
	}); err != nil {
		return fmt.Errorf("remove NIC %s from host (endpoint %s): %w", nicID, endpoint.Id, err)
	}

	log.G(ctx).Info("removed network endpoint from host")

	return nil
}
