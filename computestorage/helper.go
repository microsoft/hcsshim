package computestorage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/go-winio/pkg/security"
	"github.com/Microsoft/hcsshim/internal/virtdisk"
	"golang.org/x/sys/windows"
)

// SetupContainerBaseLayer is a helper function to setup a containers base layer. The size of
// the template VHDs is configurable with the sizeInGB parameter.
func SetupContainerBaseLayer(ctx context.Context, layerPath, vhdName string, sizeInGB uint64) error {
	var (
		baseVhdPath = filepath.Join(layerPath, vhdName+"-base.vhdx")
		diffVhdPath = filepath.Join(layerPath, vhdName+".vhdx")
		hivesPath   = filepath.Join(layerPath, "Hives")
		layoutPath  = filepath.Join(layerPath, "Layout")
	)

	if _, err := os.Stat(hivesPath); err == nil {
		os.RemoveAll(hivesPath)
	}
	if _, err := os.Stat(layoutPath); err == nil {
		os.RemoveAll(layoutPath)
	}
	if _, err := os.Stat(baseVhdPath); err == nil {
		os.RemoveAll(baseVhdPath)
	}
	if _, err := os.Stat(diffVhdPath); err == nil {
		os.RemoveAll(diffVhdPath)
	}

	createParams := &virtdisk.CreateVirtualDiskParameters{
		Version: 2,
		Version2: virtdisk.CreateVersion2{
			MaximumSize:      sizeInGB * 1024 * 1024 * 1024,
			BlockSizeInBytes: 1 * 1024 * 1024,
		},
	}

	handle, err := virtdisk.CreateVirtualDisk(ctx, baseVhdPath, virtdisk.VirtualDiskAccessFlagNone, virtdisk.CreateVirtualDiskFlagNone, createParams)
	if err != nil {
		return fmt.Errorf("failed to create VHD: %s", err)
	}

	defer func() {
		if err != nil {
			windows.CloseHandle(handle)
		}
	}()

	if err := FormatWritableLayerVhd(ctx, handle); err != nil {
		return err
	}

	if err = windows.CloseHandle(handle); err != nil {
		return fmt.Errorf("failed to close VHD handle : %s", err)
	}

	options := OsLayerOptions{
		Type: OsLayerTypeContainer,
	}

	// SetupBaseOSLayer expects an empty vhd handle for a container layer and will
	// error out otherwise.
	if err = SetupBaseOSLayer(ctx, layerPath, 0, options); err != nil {
		return err
	}

	if err = virtdisk.CreateDiffVhd(ctx, diffVhdPath, baseVhdPath); err != nil {
		return err
	}

	if err = security.GrantVmGroupAccess(baseVhdPath); err != nil {
		return fmt.Errorf("failed to grant vm group access to %s: %s", baseVhdPath, err)
	}

	if err = security.GrantVmGroupAccess(diffVhdPath); err != nil {
		return fmt.Errorf("failed to grant vm group access to %s: %s", diffVhdPath, err)
	}
	return nil
}

// SetupUtilityVMBaseLayer is a helper function to setup a UVMs base layer. The size of
// the template VHDs is configurable with the sizeInGB parameter.
func SetupUtilityVMBaseLayer(ctx context.Context, uvmPath string, vhdName string, sizeInGB uint64) error {
	baseVhdPath := filepath.Join(uvmPath, vhdName+"Base.vhdx")
	diffVhdPath := filepath.Join(uvmPath, vhdName+".vhdx")

	if _, err := os.Stat(baseVhdPath); err == nil {
		os.RemoveAll(baseVhdPath)
	}
	if _, err := os.Stat(diffVhdPath); err == nil {
		os.RemoveAll(diffVhdPath)
	}

	// Just create the vhd for utilityVM layer, no need to format it.
	createParams := &virtdisk.CreateVirtualDiskParameters{
		Version: 2,
		Version2: virtdisk.CreateVersion2{
			MaximumSize:      sizeInGB * 1024 * 1024 * 1024,
			BlockSizeInBytes: 1 * 1024 * 1024,
		},
	}

	handle, err := virtdisk.CreateVirtualDisk(ctx, baseVhdPath, virtdisk.VirtualDiskAccessFlagNone, virtdisk.CreateVirtualDiskFlagNone, createParams)
	if err != nil {
		return fmt.Errorf("failed to create VHD: %s", err)
	}

	defer func() {
		if err != nil {
			windows.CloseHandle(handle)
		}
	}()

	// If it is a utilityVM layer then the base vhd must be attached when calling
	// SetupBaseOSLayer
	attachParams := &virtdisk.AttachVirtualDiskParameters{
		Version: 2,
	}

	if err := virtdisk.AttachVirtualDisk(ctx, handle, virtdisk.AttachVirtualDiskFlagNone, attachParams); err != nil {
		return err
	}

	options := OsLayerOptions{
		Type: OsLayerTypeVM,
	}

	if err = SetupBaseOSLayer(ctx, uvmPath, handle, options); err != nil {
		return err
	}

	if err = virtdisk.DetachVirtualDisk(ctx, handle); err != nil {
		return fmt.Errorf("failed to detach VHD: %s", err)
	}

	if err = windows.CloseHandle(handle); err != nil {
		return fmt.Errorf("failed to close VHD handle: %s", err)
	}

	if err = virtdisk.CreateDiffVhd(ctx, diffVhdPath, baseVhdPath); err != nil {
		return err
	}

	if err = security.GrantVmGroupAccess(baseVhdPath); err != nil {
		return fmt.Errorf("failed to grant vm group access to %s: %s", baseVhdPath, err)
	}

	if err = security.GrantVmGroupAccess(diffVhdPath); err != nil {
		return fmt.Errorf("failed to grant vm group access to %s: %s", diffVhdPath, err)
	}

	return nil
}
