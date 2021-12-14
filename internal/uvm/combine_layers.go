package uvm

import (
	"context"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// CombineLayersWCOW combines `layerPaths` with `containerRootPath` into the
// container file system.
//
// Note: `layerPaths` and `containerRootPath` are paths from within the UVM.
func (uvm *UtilityVM) CombineLayersWCOW(ctx context.Context, layerPaths []hcsschema.Layer, containerRootPath string) error {
	if uvm.operatingSystem != "windows" {
		return errNotSupported
	}
	msr := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeCombinedLayers,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings: guestresource.WCOWCombinedLayers{
				ContainerRootPath: containerRootPath,
				Layers:            layerPaths,
			},
		},
	}
	return uvm.modify(ctx, msr)
}

// CombineLayersLCOW combines `layerPaths` and optionally `scratchPath` into an
// overlay filesystem at `rootfsPath`. If `scratchPath` is empty the overlay
// will be read only.
//
// NOTE: `layerPaths`, `scrathPath`, and `rootfsPath` are paths from within the
// UVM.
func (uvm *UtilityVM) CombineLayersLCOW(ctx context.Context, containerID string, layerPaths []string, scratchPath, rootfsPath string) error {
	if uvm.operatingSystem != "linux" {
		return errNotSupported
	}

	var layers []hcsschema.Layer
	for _, l := range layerPaths {
		layers = append(layers, hcsschema.Layer{Path: l})
	}
	msr := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeCombinedLayers,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings: guestresource.LCOWCombinedLayers{
				ContainerID:       containerID,
				ContainerRootPath: rootfsPath,
				Layers:            layers,
				ScratchPath:       scratchPath,
			},
		},
	}
	return uvm.modify(ctx, msr)
}

// RemoveCombinedLayers removes the previously combined layers at `rootfsPath`.
//
// NOTE: `rootfsPath` is the path from within the UVM.
func (uvm *UtilityVM) RemoveCombinedLayersWCOW(ctx context.Context, rootfsPath string) error {
	msr := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeCombinedLayers,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings: guestresource.WCOWCombinedLayers{
				ContainerRootPath: rootfsPath,
			},
		},
	}
	return uvm.modify(ctx, msr)
}

func (uvm *UtilityVM) RemoveCombinedLayersLCOW(ctx context.Context, rootfsPath string) error {
	msr := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeCombinedLayers,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings: guestresource.LCOWCombinedLayers{
				ContainerRootPath: rootfsPath,
			},
		},
	}
	return uvm.modify(ctx, msr)
}
