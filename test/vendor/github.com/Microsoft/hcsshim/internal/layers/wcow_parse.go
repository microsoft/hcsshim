//go:build windows
// +build windows

package layers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/api/types"

	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/uvmfolder"
)

// WCOW image layers is a tagging interface that all WCOW layers MUST implement. This is
// only used so that any random struct cannot be passed as a WCOWLayers type.
type WCOWLayers interface {
	IsWCOWLayers()
}

// scratchLayerData contains data related to the container scratch. Scratch layer format
// (i.e a VHD representing a scratch layer) doesn't change much across different types of
// read-only layers (i.e WCIFS, CIMFS etc.) so this common struct is used across all other
// layer types.
//
// Even though we can simply replace `scratchLayerData` with `scratchLayerPath`
// everywhere, it is a bit convenient to have `scratchLayerData` struct. It implements the
// `WCOWLayers` interface so that we don't have to add it for every other layer
// type. Plus, in the future if we need to include more information for some other type of
// scratch layers we can just add it to this struct.
type scratchLayerData struct {
	// Path to the scratch layer. (In most of the cases this will be a path to the
	// directory which contains the scratch vhd, however, in future this could be
	// volume or a directory that is already setup for writing)
	scratchLayerPath string
}

func (scratchLayerData) IsWCOWLayers() {}

// Legacy WCIFS based layers. Can be used for process isolated as well as hyperv isolated
// containers.
type wcowWCIFSLayers struct {
	scratchLayerData
	// layer paths in order [layerN (top-most), layerN-1,..layer0 (base)]
	layerPaths []string
}

// Represents a single forked CIM based layer. In case of a CimFS layer, most of the layer
// files are stored inside the CIM. However, some files (like registry hives) are still
// stored in the layer directory.
type forkedCIMLayer struct {
	// Path to the layer directory
	layerPath string
	// Path to the layer CIM
	cimPath string
}

// Represents CIM layers where each layer CIM is forked from its parent layer
// CIM. Currently can only be used for process isolated containers.
type wcowForkedCIMLayers struct {
	scratchLayerData
	// layer paths in order [layerN (top-most), layerN-1,..layer0 (base)]
	layers []forkedCIMLayer
}

func parseForkedCimMount(m *types.Mount) (*wcowForkedCIMLayers, error) {
	parentLayerPaths, err := getOptionAsArray(m, parentLayerPathsFlag)
	if err != nil {
		return nil, err
	}
	parentCimPaths, err := getOptionAsArray(m, parentLayerCimPathsFlag)
	if err != nil {
		return nil, err
	}
	if len(parentLayerPaths) != len(parentCimPaths) {
		return nil, fmt.Errorf("invalid mount, number of parent layer paths & cim paths should be same")
	}
	forkedCimLayers := []forkedCIMLayer{}
	for i := 0; i < len(parentCimPaths); i++ {
		forkedCimLayers = append(forkedCimLayers, forkedCIMLayer{
			layerPath: parentLayerPaths[i],
			cimPath:   parentCimPaths[i],
		})
	}
	return &wcowForkedCIMLayers{
		scratchLayerData: scratchLayerData{
			scratchLayerPath: m.Source,
		},
		layers: forkedCimLayers,
	}, nil
}

// ParseWCOWLayers parses the layers provided by containerd into the format understood by hcsshim and prepares
// them for mounting.
func ParseWCOWLayers(rootfs []*types.Mount, layerFolders []string) (WCOWLayers, error) {
	if err := validateRootfsAndLayers(rootfs, layerFolders); err != nil {
		return nil, err
	}

	if len(layerFolders) > 0 {
		return &wcowWCIFSLayers{
			scratchLayerData: scratchLayerData{
				scratchLayerPath: layerFolders[len(layerFolders)-1],
			},
			layerPaths: layerFolders[:len(layerFolders)-1],
		}, nil
	}

	m := rootfs[0]
	switch m.Type {
	case LegacyMountType:
		parentLayers, err := getOptionAsArray(m, parentLayerPathsFlag)
		if err != nil {
			return nil, err
		}
		return &wcowWCIFSLayers{
			scratchLayerData: scratchLayerData{
				scratchLayerPath: m.Source,
			},
			layerPaths: parentLayers,
		}, nil
	case CimFSMountType:
		return parseForkedCimMount(m)
	default:
		return nil, fmt.Errorf("invalid windows mount type: '%s'", m.Type)
	}
}

// GetWCOWUVMBootFilesFromLayers prepares the UVM boot files from the rootfs or layerFolders.
func GetWCOWUVMBootFilesFromLayers(ctx context.Context, rootfs []*types.Mount, layerFolders []string) (*uvm.WCOWBootFiles, error) {
	var parentLayers []string
	var scratchLayer string
	var err error

	if err = validateRootfsAndLayers(rootfs, layerFolders); err != nil {
		return nil, err
	}

	if len(layerFolders) > 0 {
		parentLayers = layerFolders[:len(layerFolders)-1]
		scratchLayer = layerFolders[len(layerFolders)-1]
	} else {
		m := rootfs[0]
		switch m.Type {
		case LegacyMountType:
			parentLayers, err = getOptionAsArray(m, parentLayerPathsFlag)
			if err != nil {
				return nil, err
			}
			scratchLayer = m.Source
		default:
			return nil, fmt.Errorf("mount type '%s' is not supported for UVM boot", m.Type)
		}
	}

	uvmFolder, err := uvmfolder.LocateUVMFolder(ctx, parentLayers)
	if err != nil {
		return nil, fmt.Errorf("failed to locate utility VM folder from layer folders: %w", err)
	}

	// In order for the UVM sandbox.vhdx not to collide with the actual
	// nested Argon sandbox.vhdx we append the \vm folder to the last
	// entry in the list.
	scratchLayer = filepath.Join(scratchLayer, "vm")
	scratchVHDPath := filepath.Join(scratchLayer, "sandbox.vhdx")
	if err = os.MkdirAll(scratchLayer, 0777); err != nil {
		return nil, err
	}

	if _, err = os.Stat(scratchVHDPath); os.IsNotExist(err) {
		sourceScratch := filepath.Join(uvmFolder, `UtilityVM\SystemTemplate.vhdx`)
		if err := copyfile.CopyFile(ctx, sourceScratch, scratchVHDPath, true); err != nil {
			return nil, err
		}
	}
	return &uvm.WCOWBootFiles{
		OSFilesPath:           filepath.Join(uvmFolder, `UtilityVM\Files`),
		OSRelativeBootDirPath: `\EFI\Microsoft\Boot`,
		ScratchVHDPath:        scratchVHDPath,
	}, nil
}
