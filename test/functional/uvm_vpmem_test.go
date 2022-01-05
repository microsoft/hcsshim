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

// Test_VPMEM tests adding/removing VPMem Read-Only layers from a v2 Linux utility VM
func Test_VPMEM(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)
	client, ctx := newCtrdClient(context.Background(), t)
	alpineLayers := testutilities.LayerFoldersPlatform(ctx, t, client, testutilities.ImageLinuxAlpineLatest, testutilities.PlatformLinux)

	u := testutilities.CreateLCOWUVMFromOpts(ctx, t, client, getDefaultLCOWUvmOptions(t, t.Name()))
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
