//go:build windows
// +build windows

package layers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/api/types"

	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/uvmfolder"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/Microsoft/hcsshim/pkg/cimfs"
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

// Represents CIM layers where each layer is stored in a block device or in a single file
// and multiple such layer CIMs are merged before mounting them. Currently can only be
// used for process isolated containers.
type wcowBlockCIMLayers struct {
	scratchLayerData
	// parent layers in order [layerN (top-most), layerN-1,..layer0 (base)]
	parentLayers []*cimfs.BlockCIM
	// a merged layer is prepared by combining all parent layers
	mergedLayer *cimfs.BlockCIM
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

// TODO(ambarve): The code to parse a mount type should be in a separate package/module
// somewhere and then should be consumed by both hcsshim & containerd from there.
func parseBlockCIMMount(m *types.Mount) (*wcowBlockCIMLayers, error) {
	var (
		parentPaths   []string
		layerType     cimfs.BlockCIMType
		mergedCIMPath string
	)

	for _, option := range m.Options {
		if val, ok := strings.CutPrefix(option, parentLayerCimPathsFlag); ok {
			err := json.Unmarshal([]byte(val), &parentPaths)
			if err != nil {
				return nil, err
			}
		} else if val, ok = strings.CutPrefix(option, blockCIMTypeFlag); ok {
			switch val {
			case "device":
				layerType = cimfs.BlockCIMTypeDevice
			case "file":
				layerType = cimfs.BlockCIMTypeSingleFile
			default:
				return nil, fmt.Errorf("invalid block CIM type `%s`", val)
			}
		} else if val, ok = strings.CutPrefix(option, mergedCIMPathFlag); ok {
			mergedCIMPath = val
		}
	}

	if len(parentPaths) == 0 {
		return nil, fmt.Errorf("need at least 1 parent layer")
	}
	if layerType == cimfs.BlockCIMTypeNone {
		return nil, fmt.Errorf("BlockCIM type not provided")
	}
	if mergedCIMPath == "" && len(parentPaths) > 1 {
		return nil, fmt.Errorf("merged CIM path not provided")
	}

	var (
		parentLayers []*cimfs.BlockCIM
		mergedLayer  *cimfs.BlockCIM
	)

	if len(parentPaths) > 1 {
		// for single parent layers merge won't be done
		mergedLayer = &cimfs.BlockCIM{
			Type:      layerType,
			BlockPath: filepath.Dir(mergedCIMPath),
			CimName:   filepath.Base(mergedCIMPath),
		}
	}

	for _, p := range parentPaths {
		parentLayers = append(parentLayers, &cimfs.BlockCIM{
			Type:      layerType,
			BlockPath: filepath.Dir(p),
			CimName:   filepath.Base(p),
		})
	}

	return &wcowBlockCIMLayers{
		scratchLayerData: scratchLayerData{
			scratchLayerPath: m.Source,
		},
		parentLayers: parentLayers,
		mergedLayer:  mergedLayer,
	}, nil
}

// ParseWCOWLayers parses the layers provided by containerd into the format understood by
// hcsshim and prepares them for mounting.
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
	case legacyMountType:
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
	case forkedCIMMountType:
		return parseForkedCimMount(m)
	case blockCIMMountType:
		return parseBlockCIMMount(m)
	default:
		return nil, fmt.Errorf("invalid windows mount type: '%s'", m.Type)
	}
}

func getVmbFSBootFiles(ctx context.Context, scratchLayer string, parentLayers []string) (*uvm.WCOWBootFiles, error) {
	uvmFolder, err := uvmfolder.LocateUVMFolder(ctx, parentLayers)
	if err != nil {
		return nil, fmt.Errorf("failed to locate utility VM folder from layer folders: %w", err)
	}

	// In order for the UVM sandbox.vhdx not to collide with the actual
	// nested Argon sandbox.vhdx we append the \vm folder to the last
	// entry in the list.
	// TODO(ambarve): This probably is a bug, we should make a separate `vm` directory
	// only if we are creating a standalone task. If we are starting a separate pod,
	// containerd snapshotter already creates a separate directory for the pod. We
	// won't have to worry about the name collision in that case. Keeping this for
	// backward compatibility/avoid breaking existing code.
	scratchLayer = filepath.Join(scratchLayer, "vm")
	scratchVHDPath := filepath.Join(scratchLayer, "sandbox.vhdx")
	if err = os.MkdirAll(scratchLayer, 0777); err != nil {
		return nil, err
	}

	if _, err = os.Stat(scratchVHDPath); os.IsNotExist(err) {
		sourceScratch := filepath.Join(uvmFolder, wclayer.UtilityVMPath, wclayer.UtilityVMScratchVhd)
		if err := copyfile.CopyFile(ctx, sourceScratch, scratchVHDPath, true); err != nil {
			return nil, err
		}
	}

	return &uvm.WCOWBootFiles{
		BootType: uvm.VmbFSBoot,
		VmbFSFiles: &uvm.VmbFSBootFiles{
			OSFilesPath:           filepath.Join(uvmFolder, wclayer.UtilityVMFilesPath),
			OSRelativeBootDirPath: wclayer.BootDirRelativePath,
			ScratchVHDPath:        scratchVHDPath,
		},
	}, nil
}

func getBlockCIMBootFiles(ctx context.Context, wl *wcowBlockCIMLayers) (*uvm.WCOWBootFiles, error) {
	scratchVHDPath := filepath.Join(wl.scratchLayerPath, "sandbox.vhdx")

	// block CIM based layers don't support multiple layers in the UVM, pick the base layer and continue
	efiVHDPath := filepath.Join(filepath.Dir(wl.parentLayers[0].BlockPath), "boot.vhd")

	return &uvm.WCOWBootFiles{
		BootType: uvm.BlockCIMBoot,
		BlockCIMFiles: &uvm.BlockCIMBootFiles{
			BootCIMVHDPath: wl.parentLayers[0].BlockPath, // This should be a block CIM with VHD footer attached.
			EFIVHDPath:     efiVHDPath,
			ScratchVHDPath: scratchVHDPath,
		},
	}, nil
}

// GetWCOWUVMBootFilesFromLayers prepares the UVM boot files from the rootfs or layerFolders.
func GetWCOWUVMBootFilesFromLayers(ctx context.Context, rootfs []*types.Mount, layerFolders []string) (*uvm.WCOWBootFiles, error) {
	var err error

	parsedWCOWLayers, err := ParseWCOWLayers(rootfs, layerFolders)
	if err != nil {
		return nil, err
	}

	switch wl := parsedWCOWLayers.(type) {
	case *wcowWCIFSLayers:
		return getVmbFSBootFiles(ctx, wl.scratchLayerPath, wl.layerPaths)
	case *wcowBlockCIMLayers:
		return getBlockCIMBootFiles(ctx, wl)
	default:
		return nil, fmt.Errorf("unsupported layer format for UVM boot: %T", parsedWCOWLayers)
	}
}
