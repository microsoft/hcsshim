// Package resources handles creating, updating, and releasing resources
// on a container
package resources

import (
	"context"
	"errors"

	"github.com/Microsoft/hcsshim/internal/credentials"
	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/uvm"
)

// NetNS returns the network namespace for the container
func (r *Resources) NetNS() string {
	return r.netNS
}

// SetNetNS updates the container resource's NetNS
func (r *Resources) SetNetNS(netNS string) {
	r.netNS = netNS
}

// SetCreatedNetNS updates the container resource's CreatedNetNS value
func (r *Resources) SetCreatedNetNS(created bool) {
	r.createdNetNS = true
}

// ContainerRootInUVM returns the containerRootInUVM for the container
func (r *Resources) ContainerRootInUVM() string {
	return r.containerRootInUVM
}

// SetContainerRootInUVM updates the container resource's containerRootInUVM value
func (r *Resources) SetContainerRootInUVM(containerRootInUVM string) {
	r.containerRootInUVM = containerRootInUVM
}

// SetAddedNetNSToVM updates the container resource's AddedNetNSToVM value
func (r *Resources) SetAddedNetNSToVM(addedNetNSToVM bool) {
	r.addedNetNSToVM = addedNetNSToVM
}

// SetLayers updates the container resource's image layers
func (r *Resources) SetLayers(l *layers.ImageLayers) {
	r.layers = l
}

// Add adds one or more resource closers to the resources struct to be
// tracked for release later on
func (r *Resources) Add(newResources ...ResourceCloser) {
	r.resources = append(r.resources, newResources...)
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
	layers *layers.ImageLayers
	// resources is a slice of the resources associated with a container
	resources []ResourceCloser
}

// ResourceCloser is a generic interface for the releasing of a resource. If a resource implements
// this interface(which they all should), freeing of that resource should entail one call to
// <resourceName>.Release(ctx)
type ResourceCloser interface {
	Release(context.Context) error
}

// NewContainerResources returns a new empty container Resources struct with the
// given container id
func NewContainerResources(id string) *Resources {
	return &Resources{
		id: id,
	}
}

// ReleaseResources releases/frees all of the resources associated with a container. This includes
// Plan9 shares, vsmb mounts, pipe mounts, network endpoints, scsi mounts, vpci devices and layers.
// TODO: make method on Resources struct.
func ReleaseResources(ctx context.Context, r *Resources, vm *uvm.UtilityVM, all bool) error {
	if vm != nil {
		if r.addedNetNSToVM {
			if err := vm.TearDownNetworking(ctx, r.netNS); err != nil {
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
		case *credentials.CCGResource:
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
