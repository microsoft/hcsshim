package hcsoci

import (
	"context"
	"errors"
	"os"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ospath"
	"github.com/Microsoft/hcsshim/internal/uvm"
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
// it in a call to ReleaseResources to ensure everything is cleaned up when a
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
	netNS              string
	// createNetNS indicates if the network namespace has been created
	createdNetNS bool
	// addedNetNSToVM indicates if the network namespace has been added to the containers utility VM
	addedNetNSToVM bool
	// layers is a pointer to a struct of the layers paths of a container
	layers *ImageLayers
	// resources is an array of the resources associated with a container
	resources []ResourceCloser
}

// ResourceCloser is a generic interface for the releasing of a resource. If a resource implements
// this interface(which they all should), freeing of that resource should entail one call to
// <resourceName>.Release(ctx)
type ResourceCloser interface {
	Release(context.Context) error
}

// AutoManagedVHD struct representing a VHD that will be cleaned up automatically.
type AutoManagedVHD struct {
	hostPath string
}

// Release removes the vhd.
func (vhd *AutoManagedVHD) Release(ctx context.Context) error {
	if err := os.Remove(vhd.hostPath); err != nil {
		log.G(ctx).WithField("hostPath", vhd.hostPath).WithError(err).Error("failed to remove automanage-virtual-disk")
	}
	return nil
}

// ReleaseResources releases/frees all of the resources associated with a container. This includes
// Plan9 shares, vsmb mounts, pipe mounts, network endpoints, scsi mounts, vpci devices and layers.
// TODO: make method on Resources struct.
func ReleaseResources(ctx context.Context, r *Resources, vm *uvm.UtilityVM, all bool) error {
	if vm != nil {
		if r.addedNetNSToVM {
			if err := vm.RemoveNetNS(ctx, r.netNS); err != nil {
				log.G(ctx).Warn(err)
			}
			r.addedNetNSToVM = false
		}
	}

	releaseErr := false
	// Release resources in reverse order so that the most recently
	// added are cleaned up first. We don't return an error right away
	// so that other resources still get cleaned up in the case of one
	// or more failing.
	for i := len(r.resources) - 1; i >= 0; i-- {
		switch r.resources[i].(type) {
		case *uvm.NetworkEndpoints:
			if r.createdNetNS {
				if err := r.resources[i].Release(ctx); err != nil {
					log.G(ctx).WithError(err).Error("failed to release container resource")
					releaseErr = true
				}
				r.createdNetNS = false
			}
		case *CCGInstance:
			if err := r.resources[i].Release(ctx); err != nil {
				log.G(ctx).WithError(err).Error("failed to release container resource")
				releaseErr = true
			}
		default:
			// Don't need to check if vm != nil here anymore as they wouldnt
			// have been added in the first place. All resources have embedded
			// vm they belong to.
			if all {
				if err := r.resources[i].Release(ctx); err != nil {
					log.G(ctx).WithError(err).Error("failed to release container resource")
					releaseErr = true
				}
			}
		}
	}
	r.resources = nil
	if releaseErr {
		return errors.New("failed to release one or more container resources")
	}

	// cleanup container state
	if vm != nil {
		if vm.DeleteContainerStateSupported() {
			if err := vm.DeleteContainerState(ctx, r.id); err != nil {
				log.G(ctx).WithError(err).Error("failed to delete container state")
			}
		}
	}

	if r.layers != nil {
		// TODO dcantah: Either make it so layers doesn't rely on the all bool for cleanup logic
		// or find a way to factor out the all bool in favor of something else.
		if err := r.layers.Release(ctx, all); err != nil {
			return err
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
