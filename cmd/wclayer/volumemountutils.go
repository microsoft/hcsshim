//go:build windows

package main

// Simple wrappers around SetVolumeMountPoint and DeleteVolumeMountPoint

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Mount volumePath (in format '\\?\Volume{GUID}' at targetPath.
// https://docs.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-setvolumemountpointw
func setVolumeMountPoint(targetPath string, volumePath string) error {
	if !strings.HasPrefix(volumePath, "\\\\?\\Volume{") {
		return fmt.Errorf("unable to mount non-volume path %s", volumePath)
	}

	// Both must end in a backslash
	slashedTarget := filepath.Clean(targetPath) + string(filepath.Separator)
	slashedVolume := volumePath + string(filepath.Separator)

	targetP, err := windows.UTF16PtrFromString(slashedTarget)
	if err != nil {
		return fmt.Errorf("unable to utf16-ise %s: %w", slashedTarget, err)
	}

	volumeP, err := windows.UTF16PtrFromString(slashedVolume)
	if err != nil {
		return fmt.Errorf("unable to utf16-ise %s: %w", slashedVolume, err)
	}

	if err := windows.SetVolumeMountPoint(targetP, volumeP); err != nil {
		return fmt.Errorf("failed calling SetVolumeMount(%q, %q): %w", slashedTarget, slashedVolume, err)
	}

	return nil
}

// Remove the volume mount at targetPath
// https://docs.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-deletevolumemountpointa
func deleteVolumeMountPoint(targetPath string) error {
	// Must end in a backslash
	slashedTarget := filepath.Clean(targetPath) + string(filepath.Separator)

	targetP, err := windows.UTF16PtrFromString(slashedTarget)
	if err != nil {
		return fmt.Errorf("unable to utf16-ise %s: %w", slashedTarget, err)
	}

	if err := windows.DeleteVolumeMountPoint(targetP); err != nil {
		return fmt.Errorf("failed calling DeleteVolumeMountPoint(%q): %w", slashedTarget, err)
	}

	return nil
}
