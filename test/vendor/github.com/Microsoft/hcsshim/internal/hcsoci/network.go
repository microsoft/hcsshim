package hcsoci

import (
	"context"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/hns"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/sirupsen/logrus"
)

func createNetworkNamespace(ctx context.Context, coi *createOptionsInternal, r *resources.Resources) error {
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
	r.SetNetNS(netID)
	r.SetCreatedNetNS(true)
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
	r.Add(&uvm.NetworkEndpoints{EndpointIDs: endpoints, Namespace: netID})
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

// Network namespace setup is a bit different for templates and clones.
// For templates and clones we use a special network namespace ID.
// Details about this can be found in the Networking section of the late-clone wiki page.
//
// In this function we take the namespace ID of the namespace that was created for this
// UVM. We hot add the namespace (with the default ID if this is a template). We get the
// endpoints associated with this namespace and then hot add those endpoints (by changing
// their namespace IDs by the deafult IDs if it is a template).
func SetupNetworkNamespace(ctx context.Context, hostingSystem *uvm.UtilityVM, nsid string) error {
	nsidInsideUVM := nsid
	if hostingSystem.IsTemplate || hostingSystem.IsClone {
		nsidInsideUVM = uvm.DEFAULT_CLONE_NETWORK_NAMESPACE_ID
	}

	// Query endpoints with actual nsid
	endpoints, err := GetNamespaceEndpoints(ctx, nsid)
	if err != nil {
		return err
	}

	// Add the network namespace inside the UVM if it is not a clone. (Clones will
	// inherit the namespace from template)
	if !hostingSystem.IsClone {
		// Get the namespace struct from the actual nsid.
		hcnNamespace, err := hcn.GetNamespaceByID(nsid)
		if err != nil {
			return err
		}

		// All templates should have a special NSID so that it
		// will be easier to debug. Override it here.
		if hostingSystem.IsTemplate {
			hcnNamespace.Id = nsidInsideUVM
		}

		if err = hostingSystem.AddNetNS(ctx, hcnNamespace); err != nil {
			return err
		}
	}

	// If adding a network endpoint to clones or a template override nsid associated
	// with it.
	if hostingSystem.IsClone || hostingSystem.IsTemplate {
		// replace nsid for each endpoint
		for _, ep := range endpoints {
			ep.Namespace = &hns.Namespace{
				ID: nsidInsideUVM,
			}
		}
	}

	if err = hostingSystem.AddEndpointsToNS(ctx, nsidInsideUVM, endpoints); err != nil {
		// Best effort clean up the NS
		if removeErr := hostingSystem.RemoveNetNS(ctx, nsidInsideUVM); removeErr != nil {
			log.G(ctx).Warn(removeErr)
		}
		return err
	}
	return nil
}
