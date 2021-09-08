// +build functional uvmvpmem

package functional

import (
	"context"
	"os"
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
	alpineLayers := testutilities.LayerFolders(t, "alpine")

	ctx := context.Background()
	u := testutilities.CreateLCOWUVM(ctx, t, t.Name())
	defer u.Close()

	var iterations uint32 = uvm.MaxVPMEMCount

	// Use layer.vhd from the alpine image as something to add
	tempDir := testutilities.CreateTempDir(t)
	if err := copyfile.CopyFile(ctx, filepath.Join(alpineLayers[0], "layer.vhd"), filepath.Join(tempDir, "layer.vhd"), true); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

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
