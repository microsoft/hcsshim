//go:build windows

package cim

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Microsoft/go-winio/pkg/guid"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	cimfs "github.com/Microsoft/hcsshim/pkg/cimfs"
)

var cimMountNamespace guid.GUID = guid.GUID{Data1: 0x6827367b, Data2: 0xc388, Data3: 0x4e9b, Data4: [8]byte{0x96, 0x1c, 0x6d, 0x2c, 0x93, 0x6c}}

// MountForkedCimLayer mounts the cim at path `cimPath` and returns the mount location of
// that cim. The containerID is used to generate the volumeID for the volume at which
// this CIM is mounted.  containerID is used so that if the shim process crashes for any
// reason, the mounted cim can be correctly cleaned up during `shim delete` call.
func MountForkedCimLayer(ctx context.Context, cimPath, containerID string) (string, error) {
	volumeGUID, err := guid.NewV5(cimMountNamespace, []byte(containerID))
	if err != nil {
		return "", fmt.Errorf("generated cim mount GUID: %w", err)
	}

	vol, err := cimfs.Mount(cimPath, volumeGUID, hcsschema.CimMountFlagCacheFiles)
	if err != nil {
		return "", err
	}
	return vol, nil
}

// Unmounts the cim mounted at the given volume
func UnmountCimLayer(ctx context.Context, volume string) error {
	return cimfs.Unmount(volume)
}

func CleanupContainerMounts(containerID string) error {
	volumeGUID, err := guid.NewV5(cimMountNamespace, []byte(containerID))
	if err != nil {
		return fmt.Errorf("generated cim mount GUID: %w", err)
	}

	volPath := fmt.Sprintf("\\\\?\\Volume{%s}\\", volumeGUID.String())
	if _, err := os.Stat(volPath); err == nil {
		err = cimfs.Unmount(volPath)
		if err != nil {
			return err
		}
	}
	return nil
}

// LayerID provides a unique GUID for each mounted CIM volume.
func LayerID(vol string) (string, error) {
	// since each mounted volume has a unique GUID, just return the same GUID as ID
	if !strings.HasPrefix(vol, "\\\\?\\Volume{") || !strings.HasSuffix(vol, "}\\") {
		return "", fmt.Errorf("volume path %s is not in the expected format", vol)
	} else {
		return strings.TrimSuffix(strings.TrimPrefix(vol, "\\\\?\\Volume{"), "}\\"), nil
	}
}
