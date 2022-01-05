package testutil

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/mount"
)

var (
	imageLayers map[string][]string
)

func init() {
	imageLayers = make(map[string][]string)
}

func GetSnapshotterFromPlatform(platform string) string {
	if platform == PlatformWindows {
		return SnapshotterWindows
	}
	return SnapshotterLinux
}

func GetLayerFoldersFromMount(t *testing.T, m mount.Mount) (layers []string) {
	for _, option := range m.Options {
		if strings.HasPrefix(option, mount.ParentLayerPathsFlag) {
			err := json.Unmarshal([]byte(option[len(mount.ParentLayerPathsFlag):]), &layers)
			if err != nil {
				t.Fatalf("failed to unmarshal parent layer paths from mount: %v", err)
			}
		}
	}
	layers = append(layers, m.Source)
	return layers
}

func LayerFolders(ctx context.Context, t *testing.T, client *containerd.Client, image string) []string {
	return LayerFoldersPlatform(ctx, t, client, image, PlatformWindows)
}

func LayerFoldersPlatform(ctx context.Context, t *testing.T, client *containerd.Client, image, platform string) []string {
	if _, ok := imageLayers[image]; !ok {
		imageLayers[image] = getLayers(ctx, t, client, image, platform)
	}
	return imageLayers[image]
}

func getLayers(ctx context.Context, t *testing.T, client *containerd.Client, image, platform string) []string {
	cid := GetImageChainID(ctx, t, client, image, platform)
	snapshotter := GetSnapshotterFromPlatform(platform)
	ms := CreateViewSnapshot(ctx, t, client, snapshotter, cid, image+"view")
	if len(ms) != 1 {
		t.Fatalf("Rootfs does not contain exactly 1 mount for the root file system")
	}
	return GetLayerFoldersFromMount(t, ms[0])
}
