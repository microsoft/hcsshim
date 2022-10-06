//go:build windows

package internal

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/lcow"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/wclayer"

	tuvm "github.com/Microsoft/hcsshim/test/internal/uvm"
)

const lcowGlobalSVMID = "test.lcowglobalsvm"

var (
	lcowGlobalSVM        *uvm.UtilityVM
	lcowCacheScratchFile string
)

func init() {
	if hcsSystem, err := hcs.OpenComputeSystem(context.Background(), lcowGlobalSVMID); err == nil {
		_ = hcsSystem.Terminate(context.Background())
	}
}

// CreateWCOWBlankRWLayer uses HCS to create a temp test directory containing a
// read-write layer containing a disk that can be used as a containers scratch
// space. The VHD is created with VM group access
// TODO: This is wrong. Need to search the folders.
func CreateWCOWBlankRWLayer(t *testing.T, imageLayers []string) string {
	t.Helper()
	//	uvmFolder, err := LocateUVMFolder(imageLayers)
	//	if err != nil {
	//		t.Fatalf("failed to locate UVM folder from %+v: %s", imageLayers, err)
	//	}

	tempDir := t.TempDir()
	if err := wclayer.CreateScratchLayer(context.Background(), tempDir, imageLayers); err != nil {
		t.Fatalf("Failed CreateScratchLayer: %s", err)
	}
	return tempDir
}

// CreateLCOWBlankRWLayer uses an LCOW utility VM to create a blank VHDX and
// format it ext4. This can then be used as a scratch space for a container, or
// for a "service VM".
func CreateLCOWBlankRWLayer(ctx context.Context, t *testing.T) string {
	t.Helper()
	if lcowGlobalSVM == nil {
		lcowGlobalSVM = tuvm.CreateAndStartLCOW(ctx, t, lcowGlobalSVMID)
		lcowCacheScratchFile = filepath.Join(t.TempDir(), "sandbox.vhdx")
	}
	tempDir := t.TempDir()

	if err := lcow.CreateScratch(ctx, lcowGlobalSVM, filepath.Join(tempDir, "sandbox.vhdx"), lcow.DefaultScratchSizeGB, lcowCacheScratchFile); err != nil {
		t.Fatalf("failed to create EXT4 scratch for LCOW test cases: %s", err)
	}
	return tempDir
}
