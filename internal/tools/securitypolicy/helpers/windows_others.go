//go:build !windows

package helpers

import (
	"context"
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// computeWindowsImageDigests is unavailable off Windows because it relies on the
// Windows CIM APIs. Callers must supply precomputed layers and mounted_cim.
func computeWindowsImageDigests(_ context.Context, _ v1.Image) ([]string, string, error) {
	return nil, "", fmt.Errorf("computing Windows container layer digests requires running on Windows; provide precomputed layers and mounted_cim instead")
}
