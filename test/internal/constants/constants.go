package constants

import (
	"fmt"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/platforms"
)

const (
	PlatformWindows    = "windows"
	PlatformLinux      = "linux"
	SnapshotterWindows = "windows"
	SnapshotterLinux   = "windows-lcow"
)

func SnapshotterFromPlatform(platform string) (string, error) {
	p, err := platforms.Parse(platform)
	if err != nil {
		return "", err
	}
	switch p.OS {
	case PlatformWindows:
		return SnapshotterWindows, nil
	case PlatformLinux:
		return SnapshotterLinux, nil
	default:
	}
	return "", fmt.Errorf("unknown platform os %q: %v", p.OS, errdefs.ErrInvalidArgument)
}
