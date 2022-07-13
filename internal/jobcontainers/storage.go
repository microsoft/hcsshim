//go:build windows

package jobcontainers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

// fallbackRootfsFormat is the fallback location for the rootfs if file binding support isn't available.
// %s will be expanded with the container ID. Trailing backslash required for SetVolumeMountPoint and
// DeleteVolumeMountPoint
const fallbackRootfsFormat = `C:\hpc\%s\`

// defaultSiloRootfsLocation is the default location the rootfs for the container will show up
// inside of a given silo. If bind filter support isn't available the rootfs will be
// C:\hpc\<containerID>
const defaultSiloRootfsLocation = `C:\hpc\`

func (c *JobContainer) mountLayers(ctx context.Context, containerID string, s *specs.Spec, volumeMountPath string) (err error) {
	if s == nil || s.Windows == nil || s.Windows.LayerFolders == nil {
		return errors.New("field 'Spec.Windows.Layerfolders' is not populated")
	}

	// Last layer always contains the sandbox.vhdx, or 'scratch' space for the container.
	scratchFolder := s.Windows.LayerFolders[len(s.Windows.LayerFolders)-1]
	if _, err := os.Stat(scratchFolder); os.IsNotExist(err) {
		if err := os.MkdirAll(scratchFolder, 0777); err != nil {
			return fmt.Errorf("failed to auto-create container scratch folder %s: %w", scratchFolder, err)
		}
	}

	// Create sandbox.vhdx if it doesn't exist in the scratch folder.
	if _, err := os.Stat(filepath.Join(scratchFolder, "sandbox.vhdx")); os.IsNotExist(err) {
		if err := wclayer.CreateScratchLayer(ctx, scratchFolder, s.Windows.LayerFolders[:len(s.Windows.LayerFolders)-1]); err != nil {
			return fmt.Errorf("failed to CreateSandboxLayer: %w", err)
		}
	}

	if s.Root == nil {
		s.Root = &specs.Root{}
	}

	if s.Root.Path == "" {
		log.G(ctx).Debug("mounting job container storage")
		rootPath, err := layers.MountWCOWLayers(ctx, containerID, s.Windows.LayerFolders, "", volumeMountPath, nil)
		if err != nil {
			return fmt.Errorf("failed to mount job container storage: %w", err)
		}
		s.Root.Path = rootPath + "\\"
	}

	return nil
}

// setupRootfsBinding binds the copy on write volume for the container to a static path
// in the container specified by 'root'.
func (c *JobContainer) setupRootfsBinding(root, target string) error {
	if err := c.job.ApplyFileBinding(root, target, false); err != nil {
		return fmt.Errorf("failed to bind rootfs to %s: %w", root, err)
	}
	return nil
}
