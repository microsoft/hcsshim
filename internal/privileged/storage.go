package privileged

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

// Trailing backslash required for SetVolumeMountPoint and DeleteVolumeMountPoint
const sandboxMountFormat = `C:\C\%s\`

func mountLayers(ctx context.Context, s *specs.Spec) error {
	if s == nil || s.Windows == nil || s.Windows.LayerFolders == nil {
		return fmt.Errorf("field 'Spec.Windows.Layerfolders' is not populated")
	}

	scratchFolder := s.Windows.LayerFolders[len(s.Windows.LayerFolders)-1]

	if _, err := os.Stat(scratchFolder); os.IsNotExist(err) {
		if err := os.MkdirAll(scratchFolder, 0777); err != nil {
			return fmt.Errorf("failed to auto-create container scratch folder %s: %s", scratchFolder, err)
		}
	}

	// Create sandbox.vhdx if it doesn't exist in the scratch folder. It's called sandbox.vhdx
	// rather than scratch.vhdx as in the v1 schema, it's hard-coded in HCS.
	if _, err := os.Stat(filepath.Join(scratchFolder, "sandbox.vhdx")); os.IsNotExist(err) {
		if err := wclayer.CreateScratchLayer(ctx, scratchFolder, s.Windows.LayerFolders[:len(s.Windows.LayerFolders)-1]); err != nil {
			return fmt.Errorf("failed to CreateSandboxLayer %s", err)
		}
	}

	if s.Root == nil {
		s.Root = &specs.Root{}
	}

	if s.Root.Path == "" {
		log.G(ctx).Debug("mounting privileged container storage")
		containerRootPath, err := hcsoci.MountContainerLayers(ctx, s.Windows.LayerFolders, "", nil)
		if err != nil {
			return fmt.Errorf("failed to mount container storage: %s", err)
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
	}).Debug("mounting sandbox volume for privileged container")

	if _, err := os.Stat(hostPath); err != nil {
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
		return fmt.Errorf("failed to mount sandbox volume to %s on host: %s", hostPath, err)
	}
	return nil
}

// Remove volume mount point. And remove folder afterwards.
func removeSandboxMountPoint(ctx context.Context, hostPath string) error {
	log.G(ctx).WithFields(logrus.Fields{
		"hostpath": hostPath,
	}).Debug("mounting sandbox volume for privileged container")

	if err := windows.DeleteVolumeMountPoint(windows.StringToUTF16Ptr(hostPath)); err != nil {
		return err
	}
	if err := os.Remove(hostPath); err != nil {
		return fmt.Errorf("failed to remove sandbox mounted folder path %s: %s", hostPath, err)
	}
	return nil
}
