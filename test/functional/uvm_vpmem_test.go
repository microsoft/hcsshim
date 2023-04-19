//go:build windows && (functional || uvmvpmem)
// +build windows
// +build functional uvmvpmem

package functional

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	tuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

// TestVPMEM tests adding/removing VPMem Read-Only layers from a v2 Linux utility VM
func TestVPMEM(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureVPMEM)

	ctx := context.Background()
	layers := linuxImageLayers(ctx, t)
	u := tuvm.CreateAndStartLCOW(ctx, t, t.Name())
	defer u.Close()

	var iterations uint32 = uvm.MaxVPMEMCount

	// Use layer.vhd from the alpine image as something to add
	tempDir := t.TempDir()
	if err := copyfile.CopyFile(ctx, filepath.Join(layers[0], "layer.vhd"), filepath.Join(tempDir, "layer.vhd"), true); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < int(iterations); i++ {
		mount, err := u.AddVPMem(ctx, filepath.Join(tempDir, "layer.vhd"))
		if err != nil {
			t.Fatalf("AddVPMEM failed: %s", err)
		}
		t.Logf("exposed as %s", mount.GuestPath)
	}

	// Remove them all
	for i := 0; i < int(iterations); i++ {
		if err := u.RemoveVPMem(ctx, filepath.Join(tempDir, "layer.vhd")); err != nil {
			t.Fatalf("RemoveVPMEM failed: %s", err)
		}
	}
}
