package hcsoci

import (
	"context"
	"os"

	"github.com/Microsoft/hcsshim/internal/hns"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ospath"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/sirupsen/logrus"
)

const (
	scratchPath = "scratch"
	rootfsPath  = "rootfs"
)

// NetNS returns the network namespace for the container
func (r *Resources) NetNS() string {
	return r.netNS
}

// Resources is the structure returned as part of creating a container. It holds
// nothing useful to clients, hence everything is lowercased. A client would use
// it in a call to ReleaseResource to ensure everything is cleaned up when a
// container exits.
type Resources struct {
	id string
	// containerRootInUVM is the base path in a utility VM where elements relating
	// to a container are exposed. For example, the mounted filesystem; the runtime
	// spec (in the case of LCOW); overlay and scratch (in the case of LCOW).
	//
	// For WCOW, this will be under wcowRootInUVM. For LCOW, this will be under
	// lcowRootInUVM, this will also be the "OCI Bundle Path".
	containerRootInUVM string

	// layers is an array of the layer folder paths which have been mounted either on
	// the host in the case or a WCOW Argon, or in a utility VM for WCOW Xenon and LCOW.
	layers []string

	// vsmbMounts is an array of the host-paths mounted into a utility VM to support
	// (bind-)mounts into a WCOW v2 Xenon.
	vsmbMounts []string

	// pipeMounts is an array of the host-paths of named pipes mounted into a utility
	// VM.
	pipeMounts []string

	// plan9Mounts is an array of all the host paths which have been added to
	// an LCOW utility VM
	plan9Mounts []*uvm.Plan9Share

	// netNS is the network namespace
	netNS string

	// networkEndpoints is the list of network endpoints used by the container
	networkEndpoints []string

	// createNetNS indicates if the network namespace has been created
	createdNetNS bool

	// addedNetNSToVM indicates if the network namespace has been added to the containers utility VM
	addedNetNSToVM bool

	// scsiMounts is an array of the vhd's mounted into a utility VM to support
	// scsi device passthrough.
	scsiMounts []scsiMount

	// vpciDevices is an array of vpci device names
	vpciDevices []string
}

type scsiMount struct {
	// path is the host path to the vhd that is mounted.
	path string
	// autoManage if `true` means that on cleanup, the runtime should
	// automatically delete this vhd.
	autoManage bool
}

// TODO: Method on the resources?
func ReleaseResources(ctx context.Context, r *Resources, vm *uvm.UtilityVM, all bool) error {
	if vm != nil && r.addedNetNSToVM {
		if err := vm.RemoveNetNS(ctx, r.netNS); err != nil {
			log.G(ctx).Warn(err)
		}
		r.addedNetNSToVM = false
	}

	if r.createdNetNS {
		for len(r.networkEndpoints) != 0 {
			endpoint := r.networkEndpoints[len(r.networkEndpoints)-1]
			err := hns.RemoveNamespaceEndpoint(r.netNS, endpoint)
			if err != nil {
				if !os.IsNotExist(err) {
					return err
				}
				log.G(ctx).WithFields(logrus.Fields{
					"endpointID": endpoint,
					"netID":      r.NetNS(),
				}).Warn("removing endpoint from namespace: does not exist")
			}
			r.networkEndpoints = r.networkEndpoints[:len(r.networkEndpoints)-1]
		}
		r.networkEndpoints = nil
		err := hns.RemoveNamespace(r.netNS)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		r.createdNetNS = false
	}

	if vm != nil && all {
		for len(r.vsmbMounts) != 0 {
			mount := r.vsmbMounts[len(r.vsmbMounts)-1]
			if err := vm.RemoveVSMB(ctx, mount); err != nil {
				return err
			}
			r.vsmbMounts = r.vsmbMounts[:len(r.vsmbMounts)-1]
		}

		for len(r.pipeMounts) != 0 {
			mount := r.pipeMounts[len(r.pipeMounts)-1]
			if err := vm.RemovePipe(ctx, mount); err != nil {
				return err
			}
			r.pipeMounts = r.pipeMounts[:len(r.pipeMounts)-1]
		}

		for len(r.plan9Mounts) != 0 {
			mount := r.plan9Mounts[len(r.plan9Mounts)-1]
			if err := vm.RemovePlan9(ctx, mount); err != nil {
				return err
			}
			r.plan9Mounts = r.plan9Mounts[:len(r.plan9Mounts)-1]
		}

		for _, sm := range r.scsiMounts {
			if err := vm.RemoveSCSI(ctx, sm.path); err != nil {
				return err
			}
			if sm.autoManage {
				if err := os.Remove(sm.path); err != nil {
					log.G(ctx).WithError(err).Warnf("failed to remove automanage-virtual-disk at: %q", sm.path)
				}
			}
		}
		r.scsiMounts = nil
	}

	if vm != nil && vm.DeleteContainerStateSupported() {
		if err := vm.DeleteContainerState(ctx, r.id); err != nil {
			log.G(ctx).WithError(err).Error("failed to delete container state")
		}
	}

	if len(r.layers) != 0 {
		op := UnmountOperationSCSI
		if vm == nil || all {
			op = UnmountOperationAll
		}
		var crp string
		if vm != nil {
			crp = containerRootfsPath(vm, r.containerRootInUVM)
		}
		err := UnmountContainerLayers(ctx, r.layers, crp, vm, op)
		if err != nil {
			return err
		}
		r.layers = nil
	}

	if vm != nil {
		for _, name := range r.vpciDevices {
			if err := vm.RemoveDevice(ctx, name); err != nil {
				log.G(ctx).WithError(err).Warn("failed to remove VPCI device")
			}
		}
	}

	return nil
}

func containerRootfsPath(uvm *uvm.UtilityVM, rootPath string) string {
	if uvm.OS() == "windows" {
		return ospath.Join(uvm.OS(), rootPath)
	}
	return ospath.Join(uvm.OS(), rootPath, rootfsPath)
}
