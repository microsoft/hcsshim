package hcsoci

import (
	"os"

	"github.com/Microsoft/hcsshim/internal/hns"
	"github.com/Microsoft/hcsshim/uvm"
	"github.com/sirupsen/logrus"
)

type Resources struct {
	InnerID          string
	NetNS            string
	NetworkEndpoints []string
	Layers           []string
	VSMBMounts       []string
	CreatedNetNS     bool
	AddedNetNSToVM   bool
}

func ReleaseResources(r *Resources, vm *uvm.UtilityVM, lastContainer bool) error {
	if vm != nil && r.AddedNetNSToVM {
		err := vm.RemoveNetNS(r.NetNS)
		if err != nil {
			logrus.Warn(err)
		}
		r.AddedNetNSToVM = false
	}

	if r.CreatedNetNS {
		for len(r.NetworkEndpoints) != 0 {
			endpoint := r.NetworkEndpoints[len(r.NetworkEndpoints)-1]
			err := hns.RemoveNamespaceEndpoint(r.NetNS, endpoint)
			if err != nil {
				if !os.IsNotExist(err) {
					return err
				}
				logrus.Warnf("removing endpoint %s from namespace %s: does not exist", endpoint, r.NetNS)
			}
			r.NetworkEndpoints = r.NetworkEndpoints[:len(r.NetworkEndpoints)-1]
		}
		r.NetworkEndpoints = nil
		err := hns.RemoveNamespace(r.NetNS)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		r.CreatedNetNS = false
	}

	if len(r.Layers) != 0 {
		op := unmountOperationAll
		if vm != nil && lastContainer {
			op = unmountOperationSCSI
		}
		err := unmountContainerLayers(r.Layers, r.InnerID, vm, op)
		if err != nil {
			return err
		}
		r.Layers = nil
	}

	if !lastContainer {
		for len(r.VSMBMounts) != 0 {
			mount := r.VSMBMounts[len(r.VSMBMounts)-1]
			if err := vm.RemoveVSMB(mount); err != nil {
				return err
			}
			r.VSMBMounts = r.VSMBMounts[:len(r.VSMBMounts)-1]
		}
	}

	return nil
}
