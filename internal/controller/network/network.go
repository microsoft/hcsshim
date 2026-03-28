//go:build windows

package network

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/sirupsen/logrus"
)

type Controller struct {
	mu sync.Mutex

	// namespaceID is the HCN namespace ID in use after a successful Setup.
	namespaceID string

	// vmEndpoints maps nicID (ID within UVM) -> HCN endpoint.
	vmEndpoints map[string]*hcn.HostComputeEndpoint

	// netState is the current lifecycle state of the network.
	netState State

	// isNamespaceSupportedByGuest determines if network namespace is supported inside the guest
	isNamespaceSupportedByGuest bool

	// vmNetManager performs host-side NIC hot-add/remove on the UVM.
	vmNetManager vmNetworkManager

	// linuxGuestMgr performs guest-side NIC inject/remove for LCOW.
	linuxGuestMgr linuxGuestNetworkManager

	// winGuestMgr performs guest-side NIC/namespace operations for WCOW.
	winGuestMgr windowsGuestNetworkManager

	// capsProvider exposes the guest's declared capabilities.
	// Used to check IsNamespaceAddRequestSupported.
	capsProvider capabilitiesProvider
}

// New creates a ready-to-use Controller in [StateNotConfigured].
func New(
	vmNetManager vmNetworkManager,
	linuxGuestMgr linuxGuestNetworkManager,
	windowsGuestMgr windowsGuestNetworkManager,
	capsProvider capabilitiesProvider,
) *Controller {
	m := &Controller{
		vmNetManager:  vmNetManager,
		linuxGuestMgr: linuxGuestMgr,
		winGuestMgr:   windowsGuestMgr,
		capsProvider:  capsProvider,
		netState:      StateNotConfigured,
		vmEndpoints:   make(map[string]*hcn.HostComputeEndpoint),
	}

	// Cache once at construction so hot-add paths can branch without re-querying.
	if caps := capsProvider.Capabilities(); caps != nil {
		m.isNamespaceSupportedByGuest = caps.IsNamespaceAddRequestSupported()
	}

	return m
}

// Setup attaches the requested HCN namespace to the guest VM
// and hot-adds all endpoints found in that namespace.
// It must be called only once; subsequent calls return an error.
func (c *Controller) Setup(ctx context.Context, opts *SetupOptions) (err error) {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Namespace, opts.NetworkNamespace))

	c.mu.Lock()
	defer c.mu.Unlock()

	log.G(ctx).Debug("starting network setup")

	// If Setup has already been called, then error out.
	if c.netState != StateNotConfigured {
		return fmt.Errorf("cannot set up network in state %s", c.netState)
	}

	defer func() {
		if err != nil {
			// If setup fails for any reason, move to invalid so no further
			// Setup calls are accepted.
			c.netState = StateInvalid
			log.G(ctx).WithError(err).Error("network setup failed, moving to invalid state")
		}
	}()

	if opts.NetworkNamespace == "" {
		return fmt.Errorf("network namespace must not be empty")
	}

	// Validate that the provided namespace exists.
	hcnNamespace, err := hcn.GetNamespaceByID(opts.NetworkNamespace)
	if err != nil {
		return fmt.Errorf("get network namespace %s: %w", opts.NetworkNamespace, err)
	}

	// Fetch all endpoints in the namespace.
	endpoints, err := c.fetchEndpointsInNamespace(ctx, hcnNamespace)
	if err != nil {
		return fmt.Errorf("fetch endpoints in namespace %s: %w", hcnNamespace.Id, err)
	}

	// Add the namespace to the guest.
	if err = c.addNetNSInsideGuest(ctx, hcnNamespace); err != nil {
		return fmt.Errorf("add network namespace to guest: %w", err)
	}

	// Hot-add all endpoints in the namespace to the guest.
	for _, endpoint := range endpoints {
		nicGUID, err := guid.NewV4()
		if err != nil {
			return fmt.Errorf("generate NIC GUID: %w", err)
		}
		// add the nicID and endpointID to the context for trace.
		nicCtx, _ := log.WithContext(ctx, logrus.WithFields(logrus.Fields{"vm_nic_id": nicGUID.String(), "hns_endpoint_id": endpoint.Id}))

		if err = c.addEndpointToGuestNamespace(nicCtx, nicGUID.String(), endpoint, opts.PolicyBasedRouting); err != nil {
			return fmt.Errorf("add endpoint %s to guest: %w", endpoint.Name, err)
		}
	}

	c.namespaceID = hcnNamespace.Id
	c.netState = StateConfigured

	log.G(ctx).Info("network setup completed successfully")

	return nil
}

// Teardown removes all guest-side NICs and the HCN namespace from the UVM.
//
// It is idempotent: calling it when the network is already torn down or not yet
// configured is a no-op.
func (c *Controller) Teardown(ctx context.Context) error {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Namespace, c.namespaceID))

	c.mu.Lock()
	defer c.mu.Unlock()

	log.G(ctx).WithField("State", c.netState).Debug("starting network teardown")

	if c.netState == StateTornDown {
		// Teardown is idempotent, so return nil if already torn down.
		log.G(ctx).Info("network already torn down, skipping")
		return nil
	}

	if c.netState == StateNotConfigured {
		// Nothing was configured; nothing to clean up.
		log.G(ctx).Info("network not configured, skipping")
		return nil
	}

	// Remove all endpoints from the guest.
	// Use a continue-on-error strategy: attempt every NIC regardless of individual
	// failures, then collect all errors.
	var teardownErrs []error
	for nicID, endpoint := range c.vmEndpoints {
		// add the nicID and endpointID to the context for trace.
		nicCtx, _ := log.WithContext(ctx, logrus.WithFields(logrus.Fields{"vm_nic_id": nicID, "hns_endpoint_id": endpoint.Id}))

		if err := c.removeEndpointFromGuestNamespace(nicCtx, nicID, endpoint); err != nil {
			teardownErrs = append(teardownErrs, fmt.Errorf("remove endpoint %s from guest: %w", endpoint.Name, err))
			continue // continue attempting to remove other endpoints
		}

		delete(c.vmEndpoints, nicID)
	}

	if len(teardownErrs) > 0 {
		// If any errors were encountered during teardown, mark the state as invalid.
		c.netState = StateInvalid
		return errors.Join(teardownErrs...)
	}

	if err := c.removeNetNSInsideGuest(ctx, c.namespaceID); err != nil {
		// Mark the state as invalid so that we can retry teardown.
		c.netState = StateInvalid
		return fmt.Errorf("remove network namespace from guest: %w", err)
	}

	// Mark as torn down if we do not encounter any errors.
	// No further Setup or Teardown calls are allowed.
	c.netState = StateTornDown

	log.G(ctx).Info("network teardown completed successfully")

	return nil
}

// fetchEndpointsInNamespace retrieves all HCN endpoints present in
// the given namespace.
// Endpoints are sorted so that those with names ending in "eth0" appear first.
func (c *Controller) fetchEndpointsInNamespace(ctx context.Context, ns *hcn.HostComputeNamespace) ([]*hcn.HostComputeEndpoint, error) {
	log.G(ctx).Info("fetching endpoints from the network namespace")

	ids, err := hcn.GetNamespaceEndpointIds(ns.Id)
	if err != nil {
		return nil, fmt.Errorf("get endpoint IDs for namespace %s: %w", ns.Id, err)
	}
	endpoints := make([]*hcn.HostComputeEndpoint, 0, len(ids))
	for _, id := range ids {
		ep, err := hcn.GetEndpointByID(id)
		if err != nil {
			return nil, fmt.Errorf("get endpoint %s: %w", id, err)
		}
		endpoints = append(endpoints, ep)
	}

	// Ensure the endpoint named "eth0" is added first when multiple endpoints are present,
	// so it maps to eth0 inside the pod network namespace within guest.
	// CNI results aren't available here, so we rely on the endpoint name suffix as a heuristic.
	cmp := func(a, b *hcn.HostComputeEndpoint) int {
		if strings.HasSuffix(a.Name, "eth0") {
			return -1
		}
		if strings.HasSuffix(b.Name, "eth0") {
			return 1
		}
		return 0
	}

	slices.SortStableFunc(endpoints, cmp)

	log.G(ctx).Tracef("fetched endpoints from the network namespace %+v", endpoints)

	return endpoints, nil
}
