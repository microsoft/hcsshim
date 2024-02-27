//go:build windows
// +build windows

package layers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/containerd/containerd/api/types"
	"github.com/containerd/errdefs"
)

// validateRootfsAndLayers checks to ensure we have appropriate information
// for setting up the container's root filesystem. It ensures the following:
// - One and only one of Rootfs or LayerFolders can be provided.
// - If LayerFolders are provided, there are at least two entries.
// - If Rootfs is provided, there is a single entry and it does not have a Target set.
func validateRootfsAndLayers(rootfs []*types.Mount, layerFolders []string) error {
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

// TODO(ambarve): functions & constants defined below are direct copies of functions already defined in
// in containerd 2.0 snapshotter/mount packages. Once we vendor containerd 2.0 in shim we can get rid of these
const (
	// parentLayerPathsFlag is the options flag used to represent the JSON encoded
	// list of parent layers required to use the layer
	parentLayerPathsFlag = "parentLayerPaths="

	// Similar to ParentLayerPathsFlag this is the optinos flag used to represent the JSON encoded list of
	// parent layer CIMs
	parentLayerCimPathsFlag = "parentCimPaths="

	LegacyMountType string = "windows-layer"
	CimFSMountType  string = "CimFS"
)

// getOptionAsArray finds if there is an option which has the given prefix and if such an
// option is found, the prefix is removed from that option string and remaining string is
// JSON unmarshalled into a string array. Note that this works because such option values
// are always stored in the form of `option_name=<marshalled JSON>`. In this case the
// optPrefix becomes `option_name=` so that remaining substring can be directly
// unmarshalled as JSON.
func getOptionAsArray(m *types.Mount, optPrefix string) ([]string, error) {
	var values []string
	for _, option := range m.Options {
		if val, ok := strings.CutPrefix(option, optPrefix); ok {
			err := json.Unmarshal([]byte(val), &values)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal option `%s`: %w", optPrefix, err)
			}
			break
		}
	}
	return values, nil
}
