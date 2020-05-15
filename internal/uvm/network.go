package uvm

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Microsoft/hcsshim/internal/ncproxyttrpc"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/hns"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/sirupsen/logrus"
)

var (
	// ErrNetNSAlreadyAttached is an error indicating the guest UVM already has
	// an endpoint by this id.
	ErrNetNSAlreadyAttached = errors.New("network namespace already added")
	// ErrNetNSNotFound is an error indicating the guest UVM does not have a
	// network namespace by this id.
	ErrNetNSNotFound = errors.New("network namespace not found")
)

// NCProxyEnabled returns if there is a network configuration client.
func (uvm *UtilityVM) NCProxyEnabled() bool {
	return uvm.ncProxyClient != nil
}

// NCProxyClient returns the network configuration proxy client.
func (uvm *UtilityVM) NCProxyClient() ncproxyttrpc.NetworkConfigProxyService {
	return uvm.ncProxyClient
}

// NetworkEndpoints is a struct containing all of the endpoint IDs of a network
// namespace.
type NetworkEndpoints struct {
	EndpointIDs []string
	// ID of the namespace the endpoints belong to
	Namespace string
}

// Release releases the resources for all of the network endpoints in a namespace.
func (endpoints *NetworkEndpoints) Release(ctx context.Context) error {
	for _, endpoint := range endpoints.EndpointIDs {
		err := hns.RemoveNamespaceEndpoint(endpoints.Namespace, endpoint)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			log.G(ctx).WithFields(logrus.Fields{
				"endpointID": endpoint,
				"netID":      endpoints.Namespace,
			}).Warn("removing endpoint from namespace: does not exist")
		}
	}
	endpoints.EndpointIDs = nil
	err := hns.RemoveNamespace(endpoints.Namespace)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// AddNetNS adds network namespace inside the guest.
//
// If a namespace with `id` already exists returns `ErrNetNSAlreadyAttached`.
func (uvm *UtilityVM) AddNetNS(ctx context.Context, id string) error {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if _, ok := uvm.namespaces[id]; ok {
		return ErrNetNSAlreadyAttached
	}

	if uvm.isNetworkNamespaceSupported() {
		// Add a Guest Network namespace. On LCOW we add the adapters
		// dynamically.
		if uvm.operatingSystem == "windows" {
			hcnNamespace, err := hcn.GetNamespaceByID(id)
			if err != nil {
				return err
			}
			guestNamespace := hcsschema.ModifySettingRequest{
				GuestRequest: guestrequest.GuestRequest{
					ResourceType: guestrequest.ResourceTypeNetworkNamespace,
					RequestType:  requesttype.Add,
					Settings:     hcnNamespace,
				},
			}
			if err := uvm.modify(ctx, &guestNamespace); err != nil {
				return err
			}
		}
	}

	if uvm.namespaces == nil {
		uvm.namespaces = make(map[string]*namespaceInfo)
	}
	uvm.namespaces[id] = &namespaceInfo{
		nics: make(map[string]*nicInfo),
	}
	return nil
}

// AddEndpointToNSWithID adds an endpoint to the network namespace with the specified
// NIC ID. If nicID is an empty string, a GUID will be generated for the ID instead.
//
// If no network namespace matches `id` returns `ErrNetNSNotFound`.
func (uvm *UtilityVM) AddEndpointToNSWithID(ctx context.Context, nsID, nicID string, endpoint *hns.HNSEndpoint) error {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	ns, ok := uvm.namespaces[nsID]
	if !ok {
		return ErrNetNSNotFound
	}
	if _, ok := ns.nics[endpoint.Id]; !ok {
		if nicID == "" {
			id, err := guid.NewV4()
			if err != nil {
				return err
			}
			nicID = id.String()
		}
		if err := uvm.addNIC(ctx, nicID, endpoint); err != nil {
			return err
		}
		ns.nics[endpoint.Id] = &nicInfo{
			ID:       nicID,
			Endpoint: endpoint,
		}
	}
	return nil
}

// AddEndpointsToNS adds all unique `endpoints` to the network namespace
// matching `id`. On failure does not roll back any previously successfully
// added endpoints.
//
// If no network namespace matches `id` returns `ErrNetNSNotFound`.
func (uvm *UtilityVM) AddEndpointsToNS(ctx context.Context, id string, endpoints []*hns.HNSEndpoint) error {
	uvm.m.Lock()
	defer uvm.m.Unlock()

	ns, ok := uvm.namespaces[id]
	if !ok {
		return ErrNetNSNotFound
	}

	for _, endpoint := range endpoints {
		if _, ok := ns.nics[endpoint.Id]; !ok {
			nicID, err := guid.NewV4()
			if err != nil {
				return err
			}
			if err := uvm.addNIC(ctx, nicID.String(), endpoint); err != nil {
				return err
			}
			ns.nics[endpoint.Id] = &nicInfo{
				ID:       nicID.String(),
				Endpoint: endpoint,
			}
		}
	}
	return nil
}

// RemoveNetNS removes the namespace from the uvm and all remaining endpoints in
// the namespace.
//
// If a namespace matching `id` is not found this command silently succeeds.
func (uvm *UtilityVM) RemoveNetNS(ctx context.Context, id string) error {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if ns, ok := uvm.namespaces[id]; ok {
		for _, ninfo := range ns.nics {
			if err := uvm.removeNIC(ctx, ninfo.ID, ninfo.Endpoint); err != nil {
				return err
			}
			ns.nics[ninfo.Endpoint.Id] = nil
		}
		// Remove the Guest Network namespace
		if uvm.isNetworkNamespaceSupported() {
			if uvm.operatingSystem == "windows" {
				hcnNamespace, err := hcn.GetNamespaceByID(id)
				if err != nil {
					return err
				}
				guestNamespace := hcsschema.ModifySettingRequest{
					GuestRequest: guestrequest.GuestRequest{
						ResourceType: guestrequest.ResourceTypeNetworkNamespace,
						RequestType:  requesttype.Remove,
						Settings:     hcnNamespace,
					},
				}
				if err := uvm.modify(ctx, &guestNamespace); err != nil {
					return err
				}
			}
		}
		delete(uvm.namespaces, id)
	}
	return nil
}

// RemoveEndpointsFromNS removes all matching `endpoints` in the network
// namespace matching `id`. If no endpoint matching `endpoint.Id` is found in
// the network namespace this command silently succeeds.
//
// If no network namespace matches `id` returns `ErrNetNSNotFound`.
func (uvm *UtilityVM) RemoveEndpointsFromNS(ctx context.Context, id string, endpoints []*hns.HNSEndpoint) error {
	uvm.m.Lock()
	defer uvm.m.Unlock()

	ns, ok := uvm.namespaces[id]
	if !ok {
		return ErrNetNSNotFound
	}

	for _, endpoint := range endpoints {
		if ninfo, ok := ns.nics[endpoint.Id]; ok && ninfo != nil {
			if err := uvm.removeNIC(ctx, ninfo.ID, ninfo.Endpoint); err != nil {
				return err
			}
			delete(ns.nics, endpoint.Id)
		}
	}
	return nil
}

// IsNetworkNamespaceSupported returns bool value specifying if network namespace is supported inside the guest
func (uvm *UtilityVM) isNetworkNamespaceSupported() bool {
	return uvm.guestCaps.NamespaceAddRequestSupported
}

func getNetworkModifyRequest(adapterID string, requestType string, settings interface{}) interface{} {
	if osversion.Get().Build >= osversion.RS5 {
		return guestrequest.NetworkModifyRequest{
			AdapterId:   adapterID,
			RequestType: requestType,
			Settings:    settings,
		}
	}
	return guestrequest.RS4NetworkModifyRequest{
		AdapterInstanceId: adapterID,
		RequestType:       requestType,
		Settings:          settings,
	}
}

// addNIC adds a nic to the Utility VM.
func (uvm *UtilityVM) addNIC(ctx context.Context, id string, endpoint *hns.HNSEndpoint) error {
	// First a pre-add. This is a guest-only request and is only done on Windows.
	if uvm.operatingSystem == "windows" {
		preAddRequest := hcsschema.ModifySettingRequest{
			GuestRequest: guestrequest.GuestRequest{
				ResourceType: guestrequest.ResourceTypeNetwork,
				RequestType:  requesttype.Add,
				Settings: getNetworkModifyRequest(
					id,
					requesttype.PreAdd,
					endpoint),
			},
		}
		if err := uvm.modify(ctx, &preAddRequest); err != nil {
			return err
		}
	}

	// Then the Add itself
	request := hcsschema.ModifySettingRequest{
		RequestType:  requesttype.Add,
		ResourcePath: fmt.Sprintf(networkResourceFormat, id),
		Settings: hcsschema.NetworkAdapter{
			EndpointId: endpoint.Id,
			MacAddress: endpoint.MacAddress,
		},
	}

	if uvm.operatingSystem == "windows" {
		request.GuestRequest = guestrequest.GuestRequest{
			ResourceType: guestrequest.ResourceTypeNetwork,
			RequestType:  requesttype.Add,
			Settings: getNetworkModifyRequest(
				id,
				requesttype.Add,
				nil),
		}
	} else {
		// Verify this version of LCOW supports Network HotAdd
		if uvm.isNetworkNamespaceSupported() {
			request.GuestRequest = guestrequest.GuestRequest{
				ResourceType: guestrequest.ResourceTypeNetwork,
				RequestType:  requesttype.Add,
				Settings: &guestrequest.LCOWNetworkAdapter{
					NamespaceID:     endpoint.Namespace.ID,
					ID:              id,
					MacAddress:      endpoint.MacAddress,
					IPAddress:       endpoint.IPAddress.String(),
					PrefixLength:    endpoint.PrefixLength,
					GatewayAddress:  endpoint.GatewayAddress,
					DNSSuffix:       endpoint.DNSSuffix,
					DNSServerList:   endpoint.DNSServerList,
					EnableLowMetric: endpoint.EnableLowMetric,
					EncapOverhead:   endpoint.EncapOverhead,
				},
			}
		}
	}

	if err := uvm.modify(ctx, &request); err != nil {
		return err
	}

	return nil
}

func (uvm *UtilityVM) removeNIC(ctx context.Context, id string, endpoint *hns.HNSEndpoint) error {
	request := hcsschema.ModifySettingRequest{
		RequestType:  requesttype.Remove,
		ResourcePath: fmt.Sprintf(networkResourceFormat, id),
		Settings: hcsschema.NetworkAdapter{
			EndpointId: endpoint.Id,
			MacAddress: endpoint.MacAddress,
		},
	}

	if uvm.operatingSystem == "windows" {
		request.GuestRequest = hcsschema.ModifySettingRequest{
			RequestType: requesttype.Remove,
			Settings: getNetworkModifyRequest(
				id,
				requesttype.Remove,
				nil),
		}
	} else {
		// Verify this version of LCOW supports Network HotRemove
		if uvm.isNetworkNamespaceSupported() {
			request.GuestRequest = guestrequest.GuestRequest{
				ResourceType: guestrequest.ResourceTypeNetwork,
				RequestType:  requesttype.Remove,
				Settings: &guestrequest.LCOWNetworkAdapter{
					NamespaceID: endpoint.Namespace.ID,
					ID:          endpoint.Id,
				},
			}
		}
	}

	if err := uvm.modify(ctx, &request); err != nil {
		return err
	}
	return nil
}
