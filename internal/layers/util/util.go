package util

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/mount"
)

// ValidateRootfsAndLayers checks to ensure we have appropriate information
// for setting up the container's root filesystem. It ensures the following:
// - One and only one of Rootfs or LayerFolders can be provided.
// - If LayerFolders are provided, there are at least two entries.
// - If Rootfs is provided, there is a single entry and it does not have a Target set.
func ValidateRootfsAndLayers(rootfs []*types.Mount, layerFolders []string) error {
	if len(rootfs) > 0 && len(layerFolders) > 0 {
		return fmt.Errorf("cannot pass both a rootfs mount and Windows.LayerFolders: %w", errdefs.ErrFailedPrecondition)
	}
	if len(rootfs) == 0 && len(layerFolders) == 0 {
		return fmt.Errorf("must pass either a rootfs mount or Windows.LayerFolders: %w", errdefs.ErrFailedPrecondition)
	}
	if len(rootfs) > 0 {
		// We have a rootfs.

		if len(rootfs) > 1 {
			return fmt.Errorf("expected a single rootfs mount: %w", errdefs.ErrFailedPrecondition)
		}
		if rootfs[0].Target != "" {
			return fmt.Errorf("rootfs mount is missing Target path: %w", errdefs.ErrFailedPrecondition)
		}
	} else {
		// We have layerFolders.

		if len(layerFolders) < 2 {
			return fmt.Errorf("must pass at least two Windows.LayerFolders: %w", errdefs.ErrFailedPrecondition)
		}
	}

	return nil
}

// ParseLegacyRootfsMount parses the rootfs mount format that we have traditionally
// used for both Linux and Windows containers.
// The mount format consists of:
//   - The scratch folder path in m.Source, which contains sandbox.vhdx.
//   - A mount option in the form parentLayerPaths=<JSON>, where JSON is an array of
//     string paths to read-only layer directories. The exact contents of these layer
//     directories are intepreteted differently for Linux and Windows containers.
func ParseLegacyRootfsMount(m *types.Mount) (string, []string, error) {
	// parentLayerPaths are passed in layerN, layerN-1, ..., layer 0
	//
	// The OCI spec expects:
	//   layerN, layerN-1, ..., layer0, scratch
	var parentLayerPaths []string
	for _, option := range m.Options {
		if strings.HasPrefix(option, mount.ParentLayerPathsFlag) {
			err := json.Unmarshal([]byte(option[len(mount.ParentLayerPathsFlag):]), &parentLayerPaths)
			if err != nil {
				return "", nil, fmt.Errorf("unmarshal parent layer paths from mount: %v: %w", err, errdefs.ErrFailedPrecondition)
			}
			// Would perhaps be worthwhile to check for unrecognized options and return an error,
			// but since this is a legacy layer mount we don't do that to avoid breaking anyone.
			break
		}
	}
	return m.Source, parentLayerPaths, nil
}

// LocateUVMFolder searches a set of layer folders to determine the "uppermost"
// layer which has a utility VM image. The order of the layers is (for historical) reasons
// Read-only-layers followed by an optional read-write layer. The RO layers are in reverse
// order so that the upper-most RO layer is at the start, and the base OS layer is the
// end.
func LocateUVMFolder(ctx context.Context, layerFolders []string) (string, error) {
	var uvmFolder string
	index := 0
	for _, layerFolder := range layerFolders {
		_, err := os.Stat(filepath.Join(layerFolder, `UtilityVM`))
		if err == nil {
			uvmFolder = layerFolder
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		index++
	}
	if uvmFolder == "" {
		return "", fmt.Errorf("utility VM folder could not be found in layers")
	}

	return uvmFolder, nil
}
