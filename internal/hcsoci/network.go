package hcsoci

import (
	"github.com/Microsoft/hcsshim/internal/hns"
	"github.com/sirupsen/logrus"
)

func createNetworkNamespace(coi *createOptionsInternal, resources *Resources) error {
	netID, err := hns.CreateNamespace()
	if err != nil {
		return err
	}
	logrus.Infof("created network namespace %s for %s", netID, coi.ID)
	resources.netNS = netID
	resources.createdNetNS = true
	for _, endpointID := range coi.Spec.Windows.Network.EndpointList {
		err = hns.AddNamespaceEndpoint(netID, endpointID)
		if err != nil {
			return err
		}
		logrus.Infof("added network endpoint %s to namespace %s", endpointID, netID)
		resources.networkEndpoints = append(resources.networkEndpoints, endpointID)
	}
	return nil
}