package testutilities

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/containerd/containerd/mount"
)

var (
	imageLayers map[string][]string
	chainIDRe   = regexp.MustCompile(`image chain ID:\s+([a-z0-9:]+)\n`)
)

func init() {
	imageLayers = make(map[string][]string)
}

func getSnapshotterName(platform string) string {
	if platform == PlatformWindows {
		return SnapshotterWindows
	}
	return SnapshotterLinux
}

func LayerFolders(t *testing.T, imageName string) []string {
	return LayerFoldersPlatform(t, imageName, PlatformWindows)
}

func LayerFoldersPlatform(t *testing.T, imageName, platform string) []string {
	if _, ok := imageLayers[imageName]; !ok {
		imageLayers[imageName] = getLayers(t, imageName, platform)
	}
	return imageLayers[imageName]
}

func getLayers(t *testing.T, imageName, platform string) []string {
	var out bytes.Buffer
	snapshotter := getSnapshotterName(platform)
	pullCmd := CtrCommand("images",
		"pull",
		"--snapshotter",
		snapshotter,
		"view",
		"--mounts",
		imageName)
	pullCmd.Stdout = &out
	if err := pullCmd.Run(); err != nil {
		t.Skipf("Failed to pull image %q with %v. Command was %v", imageName, err, pullCmd)
	}
	ms := chainIDRe.FindAllStringSubmatch(out.String(), -1)
	if len(ms) != 1 {
		t.Skipf("Failed to find chain id in output, matches are %q", ms)
	}
	chainID := ms[0][1]
	viewName := chainID + "View"

	cmd := CtrCommand("snapshot",
		"--snapshotter",
		snapshotter,
		"view",
		"--mounts",
		viewName,
		chainID)

	out.Reset()
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Skipf("Failed to find layers for %q with %v. Command was %v", chainID, err, cmd)
	}
	defer removeSnapshot(t, viewName, snapshotter)

	mounts := []mount.Mount{}
	if err := json.Unmarshal(out.Bytes(), &mounts); err != nil {
		t.Skipf("Failed to parse layers for snapshot %q", chainID)
	}

	if len(mounts) != 1 {
		t.Skip("Rootfs does not contain exactly 1 mount for the root file system")
	}

	// setup layer folders
	m := mounts[0]
	var layerFolders []string
	for _, option := range m.Options {
		if strings.HasPrefix(option, mount.ParentLayerPathsFlag) {
			err := json.Unmarshal([]byte(option[len(mount.ParentLayerPathsFlag):]), &layerFolders)
			if err != nil {
				t.Skipf("failed to unmarshal parent layer paths from mount: %v", err)
			}
		}
	}
	layerFolders = append(layerFolders, m.Source)
	return layerFolders
}

func removeSnapshot(t *testing.T, image, snapshotter string) {
	cmd := CtrCommand("snapshot",
		"--snapshotter",
		snapshotter,
		"rm",
		image)

	if err := cmd.Run(); err != nil {
		t.Logf("Could not remove image %q: %v", image, err)
	}
}
