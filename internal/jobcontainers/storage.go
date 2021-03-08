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
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

// Trailing backslash required for SetVolumeMountPoint and DeleteVolumeMountPoint
const sandboxMountFormat = `C:\C\%s\`

func mountLayers(ctx context.Context, s *specs.Spec) error {
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
		containerRootPath, err := layers.MountContainerLayers(ctx, s.Windows.LayerFolders, "", nil)
		if err != nil {
			return errors.Wrap(err, "failed to mount container storage")
		}
		s.Root.Path = containerRootPath
	}
	return nil
}

// Mount the sandbox vhd to a user friendly path.
func mountSandboxVolume(ctx context.Context, hostPath, volumeName string) (err error) {
	log.G(ctx).WithFields(logrus.Fields{
		"hostpath":   hostPath,
		"volumeName": volumeName,
	}).Debug("mounting sandbox volume for job container")

	if _, err := os.Stat(hostPath); os.IsNotExist(err) {
		if err := os.MkdirAll(hostPath, 0777); err != nil {
			return err
		}
	}

	defer func() {
		if err != nil {
			os.RemoveAll(hostPath)
		}
	}()

	if err = windows.SetVolumeMountPoint(windows.StringToUTF16Ptr(hostPath), windows.StringToUTF16Ptr(volumeName)); err != nil {
		return errors.Wrapf(err, "failed to mount sandbox volume to %s on host", hostPath)
	}
	return nil
}

// Remove volume mount point. And remove folder afterwards.
func removeSandboxMountPoint(ctx context.Context, hostPath string) error {
	log.G(ctx).WithFields(logrus.Fields{
		"hostpath": hostPath,
	}).Debug("mounting sandbox volume for job container")

	if err := windows.DeleteVolumeMountPoint(windows.StringToUTF16Ptr(hostPath)); err != nil {
		return errors.Wrap(err, "failed to delete sandbox volume mount point")
	}
	if err := os.Remove(hostPath); err != nil {
		return errors.Wrapf(err, "failed to remove sandbox mounted folder path %q", hostPath)
	}
	return nil
}
