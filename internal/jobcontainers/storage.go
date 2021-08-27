package jobcontainers

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

// Trailing backslash required for SetVolumeMountPoint and DeleteVolumeMountPoint
const sandboxMountFormat = `C:\C\%s\`

func mountLayers(ctx context.Context, containerID string, s *specs.Spec, volumeMountPath string) error {
	if s == nil || s.Windows == nil || s.Windows.LayerFolders == nil {
		return errors.New("field 'Spec.Windows.Layerfolders' is not populated")
	}

	// Last layer always contains the sandbox.vhdx, or 'scratch' space for the container.
	scratchFolder := s.Windows.LayerFolders[len(s.Windows.LayerFolders)-1]
	if _, err := os.Stat(scratchFolder); os.IsNotExist(err) {
		if err := os.MkdirAll(scratchFolder, 0777); err != nil {
			return errors.Wrapf(err, "failed to auto-create container scratch folder %s", scratchFolder)
		}
	}

	// Create sandbox.vhdx if it doesn't exist in the scratch folder.
	if _, err := os.Stat(filepath.Join(scratchFolder, "sandbox.vhdx")); os.IsNotExist(err) {
		if err := wclayer.CreateScratchLayer(ctx, scratchFolder, s.Windows.LayerFolders[:len(s.Windows.LayerFolders)-1]); err != nil {
			return errors.Wrap(err, "failed to CreateSandboxLayer")
		}
	}

	if s.Root == nil {
		s.Root = &specs.Root{}
	}

	if s.Root.Path == "" {
		log.G(ctx).Debug("mounting job container storage")
		containerRootPath, err := layers.MountContainerLayers(ctx, containerID, s.Windows.LayerFolders, "", volumeMountPath, nil)
		if err != nil {
			return errors.Wrap(err, "failed to mount container storage")
		}
		s.Root.Path = containerRootPath
	}
	return nil
}
