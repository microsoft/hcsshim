package hcsoci

import (
	"context"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/hns"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/sirupsen/logrus"
)

func createNetworkNamespace(ctx context.Context, coi *createOptionsInternal, resources *Resources) error {
	op := "hcsoci::createNetworkNamespace"
	l := log.G(ctx).WithField(logfields.ContainerID, coi.ID)
	l.Debug(op + " - Begin")
	defer func() {
		l.Debug(op + " - End")
	}()

	netID, err := hns.CreateNamespace()
	if err != nil {
		return err
	}
	log.G(ctx).WithFields(logrus.Fields{
		"netID":               netID,
		logfields.ContainerID: coi.ID,
	}).Info("created network namespace for container")
	resources.netNS = netID
	resources.createdNetNS = true
	endpoints := make([]string, 0)
	for _, endpointID := range coi.Spec.Windows.Network.EndpointList {
		err = hns.AddNamespaceEndpoint(netID, endpointID)
		if err != nil {
			return err
		}
		log.G(ctx).WithFields(logrus.Fields{
			"netID":      netID,
			"endpointID": endpointID,
		}).Info("added network endpoint to namespace")
		endpoints = append(endpoints, endpointID)
	}
	resources.resources = append(resources.resources, &uvm.NetworkEndpoints{EndpointIDs: endpoints, Namespace: netID})
	return nil
}

// GetNamespaceEndpoints gets all endpoints in `netNS`
func GetNamespaceEndpoints(ctx context.Context, netNS string) ([]*hns.HNSEndpoint, error) {
	op := "hcsoci::GetNamespaceEndpoints"
	l := log.G(ctx).WithField("netns-id", netNS)
	l.Debug(op + " - Begin")
	defer func() {
		l.Debug(op + " - End")
	}()

	ids, err := hns.GetNamespaceEndpoints(netNS)
	if err != nil {
		return nil, err
	}
	var endpoints []*hns.HNSEndpoint
	for _, id := range ids {
		endpoint, err := hns.GetHNSEndpointByID(id)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints, nil
}

// SetupNetworkNamespace adds the network namespace with given nsid into
// the given UtilityVM and then adds the endpoints associated with that nsid into that
// namespace.
func SetupNetworkNamespace(ctx context.Context, hostingSystem *uvm.UtilityVM, nsid string) (err error) {
	endpoints, err := GetNamespaceEndpoints(ctx, nsid)
	if err != nil {
		return err
	}
	err = hostingSystem.AddNetNS(ctx, nsid)
	if err != nil {
		return err
	}
	err = hostingSystem.AddEndpointsToNS(ctx, nsid, endpoints)
	if err != nil {
		// Best effort clean up the NS
		hostingSystem.RemoveNetNS(ctx, nsid)
		return err
	}
	return nil
}

// Network namespace setup is a bit different for templates and clones.
// A normal network namespace creation works as follows: When a new Pod is created, HNS is
// called to create a new namespace and endpoints inside that namespace. Then we hot add
// this network namespace and then the network endpoints associated with that namespace
// into this pod UVM. Later when we create containers inside that pod they start running
// inside this namespace. The information about namespace and endpoints is maintained by
// the HNS and it is stored in the guest VM's registry (UVM) as well. So if inside the pod
// we see a namespcae with id 'NSID' then we can query HNS to find all the information
// about that namespace.
//
// When we clone a VM (with containers running inside it) we can hot add a new namespace
// and endpoints created for that namespace to the uvm but we can't make the existing
// processes/containers to automatically switch over to this new namespace. To solve this
// problem when we create a template or a cloned pod we will ask HNS to create a new
// namespace and endpoints for that UVM but when we actually send a request to hot add
// that namespace we will change the namespace ID with the default ID that is specifically
// created for cloning purposes. Similarly, when hot adding an endpoint we will modify
// this endpoint information to set its network namespace ID to this default ID. This way
// inside every template and cloned pod the namespace ID will remain same (but each cloned
// UVM will have a different endpoint) but the HNS will have the actual namespace ID that
// was created for that UVM. The reason to have a specific default ID is that it will help
// in debugging namespace related scenarios as it makes it clear that this is a cloned VM
// and hence has a namespace that was cloned from some template.
//
// In this function we take the namespace ID of the namespace that was created for this
// UVM. We hot add the namespace (with the default ID) only if this is a template (clones
// will already have this namespace). We get the endpoints associated with
// this namespace and then hot add those endpoints by changing their namespace IDs by the
// deafult IDs.
func SetupNetworkNamespaceForClones(ctx context.Context, hostingSystem *uvm.UtilityVM, nsid string, isTemplate bool) (err error) {
	endpoints, err := GetNamespaceEndpoints(ctx, nsid)
	if err != nil {
		return err
	}

	if isTemplate {
		hcnNamespace, err := hcn.GetNamespaceByID(nsid)
		if err != nil {
			return err
		}
		// override the namespce ID with the default ID
		hcnNamespace.Id = hns.CLONING_DEFAULT_NETWORK_NAMESPACE_ID

		err = hostingSystem.AddNetNSRAW(ctx, hcnNamespace)
		if err != nil {
			return err
		}
	}

	// replace nsid for each endpoint
	for _, ep := range endpoints {
		ep.Namespace = &hns.Namespace{
			ID: hns.CLONING_DEFAULT_NETWORK_NAMESPACE_ID,
		}
	}

	err = hostingSystem.AddEndpointsToNS(ctx, hns.CLONING_DEFAULT_NETWORK_NAMESPACE_ID, endpoints)
	if err != nil {
		// Best effort clean up the NS
		hostingSystem.RemoveNetNS(ctx, nsid)
		return err
	}
	return nil
}
