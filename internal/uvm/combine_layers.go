//go:build windows

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
func (uvm *UtilityVM) CombineLayersWCOW(ctx context.Context, layerPaths []hcsschema.Layer, containerRootPath string, filterType hcsschema.FileSystemFilterType, containerID string) error {
	if uvm.operatingSystem != "windows" {
		return errNotSupported
	}

	// FilterType was added to the CombinedLayers HCS schema in version 2.9.
	// Inbox GCS (vmcomputeagent.exe) on older Windows hosts (e.g. Windows
	// Server 2022) ships a pre-2.9 schema and uses a strict JSON unmarshaller
	// that rejects unknown fields with HCS_E_INVALID_JSON ("$.FilterType").
	// Since WCIFS is the default behavior on the GCS side when the field is
	// absent, drop the value here so the `omitempty` JSON tag removes it from
	// the wire format. This preserves behavior on newer GCS (which also defaults
	// to WCIFS) while remaining compatible with older inbox GCS.
	// See: https://github.com/microsoft/hcsshim/issues/2714
	if filterType == hcsschema.WCIFS {
		filterType = ""
	}

	var modifyRequest *hcsschema.ModifySettingRequest
	if uvm.HasConfidentialPolicy() {
		modifyRequest = &hcsschema.ModifySettingRequest{
			GuestRequest: guestrequest.ModificationRequest{
				ResourceType: guestresource.ResourceTypeCWCOWCombinedLayers,
				RequestType:  guestrequest.RequestTypeAdd,
				Settings: guestresource.CWCOWCombinedLayers{
					ContainerID: containerID,
					CombinedLayers: guestresource.WCOWCombinedLayers{
						ContainerRootPath: containerRootPath,
						Layers:            layerPaths,
						FilterType:        filterType,
					},
				},
			},
		}
	} else {
		modifyRequest = &hcsschema.ModifySettingRequest{
			GuestRequest: guestrequest.ModificationRequest{
				ResourceType: guestresource.ResourceTypeCombinedLayers,
				RequestType:  guestrequest.RequestTypeAdd,
				Settings: guestresource.WCOWCombinedLayers{
					ContainerRootPath: containerRootPath,
					Layers:            layerPaths,
					FilterType:        filterType,
				},
			},
		}
	}
	return uvm.modify(ctx, modifyRequest)
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
	var msr *hcsschema.ModifySettingRequest

	if uvm.HasConfidentialPolicy() {
		msr = &hcsschema.ModifySettingRequest{
			GuestRequest: guestrequest.ModificationRequest{
				ResourceType: guestresource.ResourceTypeCWCOWCombinedLayers,
				RequestType:  guestrequest.RequestTypeRemove,
				Settings: guestresource.CWCOWCombinedLayers{
					CombinedLayers: guestresource.WCOWCombinedLayers{
						ContainerRootPath: rootfsPath,
					},
				},
			},
		}
	} else {
		msr = &hcsschema.ModifySettingRequest{
			GuestRequest: guestrequest.ModificationRequest{
				ResourceType: guestresource.ResourceTypeCombinedLayers,
				RequestType:  guestrequest.RequestTypeRemove,
				Settings: guestresource.WCOWCombinedLayers{
					ContainerRootPath: rootfsPath,
				},
			},
		}
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
