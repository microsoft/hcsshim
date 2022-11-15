//go:build windows

package layers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/mount"

	testctrd "github.com/Microsoft/hcsshim/test/internal/containerd"
)

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
