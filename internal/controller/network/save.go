//go:build windows && (lcow || wcow)

package network

import (
	"context"
	"fmt"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/hcn"
	netsave "github.com/Microsoft/hcsshim/internal/controller/network/save"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// Save serializes the controller's current network state into a portable
// envelope that can be handed to a migration destination. It succeeds only
// when the network is fully configured, and on success freezes the source
// until it is resumed or torn down.
func (c *Controller) Save(ctx context.Context) (*anypb.Any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Only a fully configured network is in a stable, migratable state.
	if c.netState != StateConfigured {
		return nil, fmt.Errorf("network controller in state %s; want %s", c.netState, StateConfigured)
	}

	// Capture the scalar configuration into the snapshot.
	state := &netsave.Payload{
		SchemaVersion:               netsave.SchemaVersion,
		NamespaceID:                 c.namespaceID,
		PolicyBasedRouting:          c.policyBasedRouting,
		IsNamespaceSupportedByGuest: c.isNamespaceSupportedByGuest,
		VmEndpoints:                 make(map[string]*netsave.EndpointBinding, len(c.vmEndpoints)),
	}

	// Copy each bound endpoint so the destination can re-create the NICs.
	for nicID, ep := range c.vmEndpoints {
		if ep == nil {
			return nil, fmt.Errorf("nil endpoint bound to NIC %s", nicID)
		}
		state.VmEndpoints[nicID] = &netsave.EndpointBinding{
			EndpointID:   ep.Id,
			MacAddress:   ep.MacAddress,
			EndpointName: ep.Name,
		}
	}

	// Marshal and wrap the snapshot in a self-describing envelope.
	payload, err := proto.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal network saved state: %w", err)
	}

	// Freeze the source until the migration is resumed or torn down.
	c.netState = StateSourceMigrating

	log.G(ctx).WithField(logfields.GuestNetworkNamespaceID, c.namespaceID).Info("network controller saved state for migration")

	return &anypb.Any{TypeUrl: netsave.TypeURL, Value: payload}, nil
}

// Import reconstructs a controller from an envelope produced by [Controller.Save].
// The returned controller carries the saved state but is not yet bound to a
// running VM, so operational calls are rejected until [Controller.Resume].
func Import(ctx context.Context, env *anypb.Any) (*Controller, error) {
	// Reject an empty or mistyped envelope before touching its bytes.
	if env == nil {
		return nil, fmt.Errorf("network saved-state envelope is nil")
	}

	if env.GetTypeUrl() != netsave.TypeURL {
		return nil, fmt.Errorf("unsupported network saved-state type %q", env.GetTypeUrl())
	}

	// Decode and reject any payload this build cannot interpret.
	state := &netsave.Payload{}
	if err := proto.Unmarshal(env.GetValue(), state); err != nil {
		return nil, fmt.Errorf("unmarshal network saved state: %w", err)
	}

	if v := state.GetSchemaVersion(); v != netsave.SchemaVersion {
		return nil, fmt.Errorf("unsupported network saved-state schema version %d (want %d)", v, netsave.SchemaVersion)
	}

	// Rehydrate into the destination-migrating state: state is restored but no
	// live host/guest interfaces are bound, so operational calls are rejected
	// until Resume.
	c := &Controller{
		vmEndpoints: make(map[string]*hcn.HostComputeEndpoint),
		netState:    StateDestinationMigrating,
	}

	// Restore the scalar configuration.
	c.namespaceID = state.GetNamespaceID()
	c.policyBasedRouting = state.GetPolicyBasedRouting()
	c.isNamespaceSupportedByGuest = state.GetIsNamespaceSupportedByGuest()

	// Rebuild the endpoint bindings captured at save time.
	for nicID, b := range state.GetVmEndpoints() {
		if nicID == "" || b == nil {
			return nil, fmt.Errorf("invalid endpoint binding for NIC %q in saved state", nicID)
		}
		c.vmEndpoints[nicID] = &hcn.HostComputeEndpoint{
			Id:         b.GetEndpointID(),
			MacAddress: b.GetMacAddress(),
			Name:       b.GetEndpointName(),
		}
	}

	log.G(ctx).WithField(logfields.GuestNetworkNamespaceID, c.namespaceID).Info("network controller imported")

	return c, nil
}

// Patch records the destination-side namespace ID that a later
// [Controller.ResetAfterMigration] uses to rebind endpoints on the new host.
func (c *Controller) Patch(ctx context.Context, networkNamespaceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.migratedNamespaceID = networkNamespaceID

	log.G(ctx).WithFields(logrus.Fields{
		logfields.GuestNetworkNamespaceID: c.namespaceID,
		logfields.MigratedNamespaceID:     c.migratedNamespaceID,
	}).Debug("network controller patched with migrated namespace ID")
}

// Resume returns a migrating controller to the configured, operational state.
// On the destination it binds the live VM and guest; on the source it rolls the
// snapshot back, lifting the freeze that Save applied.
func (c *Controller) Resume(ctx context.Context, vm *vmmanager.UtilityVM, guest *guestmanager.Guest) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.vmNetwork = vm
	// The guest manager provides both guest-side network ops and capability checks.
	c.guestNetwork = guest
	c.capsProvider = guest
	c.netState = StateConfigured

	log.G(ctx).WithField(logfields.GuestNetworkNamespaceID, c.namespaceID).Debug("network controller resumed")
}

// ResetAfterMigration swaps the endpoints carried over from the source for the
// ones present in the destination namespace, leaving the network operational
// on the new host.
func (c *Controller) ResetAfterMigration(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Drop the stale source NICs inherited from the saved state.
	for nicID, ep := range c.vmEndpoints {
		if err := c.removeEndpointFromGuestNamespace(ctx, nicID, nil); err != nil {
			return fmt.Errorf("reset stale source NIC %s (endpoint %s): %w", nicID, ep.Id, err)
		}
		delete(c.vmEndpoints, nicID)
	}

	// Look up the destination namespace and its endpoints.
	hcnNamespace, err := hcn.GetNamespaceByID(c.migratedNamespaceID)
	if err != nil {
		return fmt.Errorf("get destination namespace %s: %w", c.migratedNamespaceID, err)
	}

	endpoints, err := c.fetchEndpointsInNamespace(ctx, hcnNamespace)
	if err != nil {
		return fmt.Errorf("fetch endpoints in destination namespace %s: %w", c.migratedNamespaceID, err)
	}

	// Add each destination endpoint to the guest under a fresh NIC ID.
	for _, endpoint := range endpoints {
		nicGUID, err := guid.NewV4()
		if err != nil {
			return fmt.Errorf("generate NIC GUID: %w", err)
		}
		if err := c.addEndpointToGuestNamespace(ctx, c.namespaceID, nicGUID.String(), endpoint, c.policyBasedRouting); err != nil {
			return fmt.Errorf("add destination endpoint %s to guest: %w", endpoint.Name, err)
		}
	}

	c.netState = StateConfigured
	c.migratedNamespaceID = ""

	log.G(ctx).WithField(logfields.GuestNetworkNamespaceID, c.namespaceID).
		Info("network reset for migration: rebound destination endpoints")

	return nil
}
