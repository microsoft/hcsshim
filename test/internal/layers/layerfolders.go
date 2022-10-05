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
func FromImage(ctx context.Context, tb testing.TB, client *containerd.Client, ref, platform, snapshotter string) []string {
	tb.Helper()
	chainID := testctrd.PullImage(ctx, tb, client, ref, platform)
	return FromChainID(ctx, tb, client, chainID, snapshotter)
}

// FromChainID returns thee layer paths of a given image chain ID
func FromChainID(ctx context.Context, tb testing.TB, client *containerd.Client, chainID, snapshotter string) []string {
	tb.Helper()
	ms := testctrd.CreateViewSnapshot(ctx, tb, client, snapshotter, chainID, chainID+"view")
	if len(ms) != 1 {
		tb.Fatalf("Rootfs does not contain exactly 1 mount for the root file system")
	}

	return FromMount(ctx, tb, ms[0])
}

// FromMount returns the layer paths of a given mount
func FromMount(_ context.Context, tb testing.TB, m mount.Mount) (layers []string) {
	tb.Helper()
	for _, option := range m.Options {
		if strings.HasPrefix(option, mount.ParentLayerPathsFlag) {
			err := json.Unmarshal([]byte(option[len(mount.ParentLayerPathsFlag):]), &layers)
			if err != nil {
				tb.Fatalf("failed to unmarshal parent layer paths from mount: %v", err)
			}
		}
	}
	layers = append(layers, m.Source)

	return layers
}

// Deprecated: This relies on docker. Use [FromChainID] or [FromMount] instead.
func LayerFolders(tb testing.TB, imageName string) []string {
	tb.Helper()
	if _, ok := imageLayers[imageName]; !ok {
		imageLayers[imageName] = getLayers(tb, imageName)
	}
	return imageLayers[imageName]
}

// Deprecated: This relies on docker. Use [FromChainID] or [FromMount] instead.
func getLayers(tb testing.TB, imageName string) []string {
	tb.Helper()
	cmd := exec.Command("docker", "inspect", imageName, "-f", `"{{.GraphDriver.Data.dir}}"`)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		tb.Skipf("Failed to find layers for %q. Check docker images", imageName)
	}
	imagePath := strings.Replace(strings.TrimSpace(out.String()), `"`, ``, -1)
	layers := getLayerChain(tb, imagePath)
	return append([]string{imagePath}, layers...)
}

// Deprecated: This relies on docker. Use [FromChainID] or [FromMount] instead.
func getLayerChain(tb testing.TB, layerFolder string) []string {
	tb.Helper()
	jPath := filepath.Join(layerFolder, "layerchain.json")
	content, err := os.ReadFile(jPath)
	if os.IsNotExist(err) {
		tb.Fatalf("layerchain not found")
	} else if err != nil {
		tb.Fatalf("failed to read layerchain")
	}

	var layerChain []string
	err = json.Unmarshal(content, &layerChain)
	if err != nil {
		tb.Fatalf("failed to unmarshal layerchain")
	}
	return layerChain
}
