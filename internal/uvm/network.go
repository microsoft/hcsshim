package uvm

import (
	"fmt"
	"path"

	"github.com/Microsoft/hcsshim/internal/guid"
	"github.com/Microsoft/hcsshim/internal/hns"
	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/sirupsen/logrus"
)

func (uvm *UtilityVM) AddNetNS(id string, endpoints []*hns.HNSEndpoint) (err error) {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	ns := uvm.namespaces[id]
	if ns == nil {
		ns = &namespaceInfo{}
		defer func() {
			if err != nil {
				if e := uvm.removeNamespaceNICs(ns); e != nil {
					logrus.Warn("failed to undo NIC add: %s", e)
				}
			}
		}()
		for _, endpoint := range endpoints {
			nicID := guid.New()
			err = uvm.addNIC(nicID, endpoint)
			if err != nil {
				return err
			}
			ns.nics = append(ns.nics, nicInfo{nicID, endpoint})
		}
		if uvm.namespaces == nil {
			uvm.namespaces = make(map[string]*namespaceInfo)
		}
		uvm.namespaces[id] = ns
	}
	ns.refCount++
	return nil
}

func (uvm *UtilityVM) RemoveNetNS(id string) error {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	ns := uvm.namespaces[id]
	if ns == nil || ns.refCount <= 0 {
		panic(fmt.Errorf("removed a namespace that was not added: %s", id))
	}
	ns.refCount--
	var err error
	if ns.refCount == 0 {
		err = uvm.removeNamespaceNICs(ns)
		delete(uvm.namespaces, id)
	}
	return err
}

func (uvm *UtilityVM) removeNamespaceNICs(ns *namespaceInfo) error {
	for len(ns.nics) != 0 {
		nic := ns.nics[len(ns.nics)-1]
		err := uvm.removeNIC(nic.ID, nic.Endpoint)
		if err != nil {
			return err
		}
		ns.nics = ns.nics[:len(ns.nics)-1]
	}
	return nil
}

func (uvm *UtilityVM) addNIC(id guid.GUID, endpoint *hns.HNSEndpoint) error {
	request := schema2.ModifySettingsRequestV2{
		ResourceType: schema2.ResourceTypeNetwork,
		RequestType:  schema2.RequestTypeAdd,
		Settings: schema2.VirtualMachinesResourcesNetworkNic{
			EndpointID: endpoint.Id,
			MacAddress: endpoint.MacAddress,
		},
		HostedSettings: endpoint,
		ResourceUri:    path.Join("VirtualMachine/Devices/NIC", id.String()),
	}
	if err := uvm.Modify(&request); err != nil {
		return err
	}
	return nil
}

func (uvm *UtilityVM) removeNIC(id guid.GUID, endpoint *hns.HNSEndpoint) error {
	request := schema2.ModifySettingsRequestV2{
		ResourceType: schema2.ResourceTypeNetwork,
		RequestType:  schema2.RequestTypeRemove,
		Settings: schema2.VirtualMachinesResourcesNetworkNic{
			EndpointID: endpoint.Id,
			MacAddress: endpoint.MacAddress,
		},
		ResourceUri: path.Join("VirtualMachine/Devices/NIC", id.String()),
	}
	if err := uvm.Modify(&request); err != nil {
		return err
	}
	return nil
}
