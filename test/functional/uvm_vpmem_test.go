//go:build functional || uvmvpmem
// +build functional uvmvpmem

package functional

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
)

// TestVPMEM tests adding/removing VPMem Read-Only layers from a v2 Linux utility VM
func TestVPMEM(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)
	client, ctx := getCtrdClient(context.Background(), t)
	alpineLayers := testutilities.LayerFolders(ctx, t, client, "alpine")

	u := testutilities.CreateLCOWUVMFromOpts(ctx, t, client, getDefaultLcowUvmOptions(t, t.Name()))
	defer u.Close()

	var iterations uint32 = uvm.MaxVPMEMCount

	// Use layer.vhd from the alpine image as something to add
	tempDir := t.TempDir()
	if err := copyfile.CopyFile(ctx, filepath.Join(alpineLayers[0], "layer.vhd"), filepath.Join(tempDir, "layer.vhd"), true); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < int(iterations); i++ {
		uvmPath, err := u.AddVPMem(ctx, filepath.Join(tempDir, "layer.vhd"))
		if err != nil {
			t.Fatalf("AddVPMEM failed: %s", err)
		}
		t.Logf("exposed as %s", uvmPath)
	}

	// Remove them all
	for i := 0; i < int(iterations); i++ {
		if err := u.RemoveVPMem(ctx, filepath.Join(tempDir, "layer.vhd")); err != nil {
			t.Fatalf("RemoveVPMEM failed: %s", err)
		}
	}
}
