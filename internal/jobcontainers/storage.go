//go:build windows

package jobcontainers

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/resources"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// fallbackRootfsFormat is the fallback location for the rootfs if file binding support isn't available.
// %s will be expanded with the container ID. Trailing backslash required for SetVolumeMountPoint and
// DeleteVolumeMountPoint
const fallbackRootfsFormat = `C:\hpc\%s\`

// defaultSiloRootfsLocation is the default location the rootfs for the container will show up
// inside of a given silo. If bind filter support isn't available the rootfs will be
// C:\hpc\<containerID>
const defaultSiloRootfsLocation = `C:\hpc\`

func (c *JobContainer) mountLayers(ctx context.Context, containerID string, s *specs.Spec, wl layers.WCOWLayers, volumeMountPath string) (_ resources.ResourceCloser, err error) {
	if s.Root == nil {
		s.Root = &specs.Root{}
	}
	if wl == nil {
		return nil, fmt.Errorf("layers can not be nil")
	}

	var closer resources.ResourceCloser
	if s.Root.Path == "" {
		var mountedLayers *layers.MountedWCOWLayers
		log.G(ctx).Debug("mounting job container storage")
		mountedLayers, closer, err = layers.MountWCOWLayers(ctx, containerID, nil, wl)
		if err != nil {
			return nil, fmt.Errorf("failed to mount job container storage: %w", err)
		}
		defer func() {
			if err != nil {
				closeErr := closer.Release(ctx)
				if closeErr != nil {
					log.G(ctx).WithError(closeErr).Errorf("failed to cleanup mounted layers during another failure(%s)", err)
				}
			}
		}()

		s.Root.Path = mountedLayers.RootFS + "\\"
	}

	if volumeMountPath != "" {
		if err = layers.MountSandboxVolume(ctx, volumeMountPath, s.Root.Path); err != nil {
			return nil, err
		}
		layerCloser := closer
		closer = resources.ResourceCloserFunc(func(ctx context.Context) error {
			unmountErr := layers.RemoveSandboxMountPoint(ctx, volumeMountPath)
			if unmountErr != nil {
				return unmountErr
			}
			return layerCloser.Release(ctx)
		})
	}

	return closer, nil
}

// setupRootfsBinding binds the copy on write volume for the container to a static path
// in the container specified by 'root'.
func (c *JobContainer) setupRootfsBinding(root, target string) error {
	if err := c.job.ApplyFileBinding(root, target, false); err != nil {
		return fmt.Errorf("failed to bind rootfs to %s: %w", root, err)
	}
	return nil
}
