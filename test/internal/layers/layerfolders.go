//go:build windows

package layers

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/mount"

	testctrd "github.com/Microsoft/hcsshim/test/internal/containerd"
)

var imageLayers map[string][]string

func init() {
	imageLayers = make(map[string][]string)
}

// FromImage returns thee layer paths of a given image, pulling it if necessary
func FromImage(ctx context.Context, t testing.TB, client *containerd.Client, ref, platform, snapshotter string) []string {
	chainID := testctrd.PullImage(ctx, t, client, ref, platform)
	return FromChainID(ctx, t, client, chainID, snapshotter)
}

// FromChainID returns thee layer paths of a given image chain ID
func FromChainID(ctx context.Context, t testing.TB, client *containerd.Client, chainID, snapshotter string) []string {
	ms := testctrd.CreateViewSnapshot(ctx, t, client, snapshotter, chainID, chainID+"view")
	if len(ms) != 1 {
		t.Fatalf("Rootfs does not contain exactly 1 mount for the root file system")
	}

	return FromMount(ctx, t, ms[0])
}

// FromMount returns the layer paths of a given mount
func FromMount(_ context.Context, t testing.TB, m mount.Mount) (layers []string) {
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

// Deprecated: This relies on docker. Use [FromChainID] or [FromMount] instead.
func LayerFolders(t testing.TB, imageName string) []string {
	if _, ok := imageLayers[imageName]; !ok {
		imageLayers[imageName] = getLayers(t, imageName)
	}
	return imageLayers[imageName]
}

func getLayers(t testing.TB, imageName string) []string {
	cmd := exec.Command("docker", "inspect", imageName, "-f", `"{{.GraphDriver.Data.dir}}"`)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Skipf("Failed to find layers for %q. Check docker images", imageName)
	}
	imagePath := strings.Replace(strings.TrimSpace(out.String()), `"`, ``, -1)
	layers := getLayerChain(t, imagePath)
	return append([]string{imagePath}, layers...)
}

func getLayerChain(t testing.TB, layerFolder string) []string {
	jPath := filepath.Join(layerFolder, "layerchain.json")
	content, err := os.ReadFile(jPath)
	if os.IsNotExist(err) {
		t.Fatalf("layerchain not found")
	} else if err != nil {
		t.Fatalf("failed to read layerchain")
	}

	var layerChain []string
	err = json.Unmarshal(content, &layerChain)
	if err != nil {
		t.Fatalf("failed to unmarshal layerchain")
	}
	return layerChain
}
