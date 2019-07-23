// +build functional uvmvsmb

package functional

import (
	"context"
	"os"
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
)

// TestVSMB tests adding/removing VSMB layers from a v2 Windows utility VM
func TestVSMB(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)
	uvm, _, uvmScratchDir := testutilities.CreateWCOWUVM(context.Background(), t, t.Name(), "microsoft/nanoserver")
	defer os.RemoveAll(uvmScratchDir)
	defer uvm.Close()

	dir := testutilities.CreateTempDir(t)
	defer os.RemoveAll(dir)
	var iterations uint32 = 64
	options := &hcsschema.VirtualSmbShareOptions{
		ReadOnly:            true,
		PseudoOplocks:       true,
		TakeBackupPrivilege: true,
		CacheIo:             true,
		ShareRead:           true,
	}
	for i := 0; i < int(iterations); i++ {
		if err := uvm.AddVSMB(context.Background(), dir, "", options); err != nil {
			t.Fatalf("AddVSMB failed: %s", err)
		}
	}

	// Remove them all
	for i := 0; i < int(iterations); i++ {
		if err := uvm.RemoveVSMB(context.Background(), dir); err != nil {
			t.Fatalf("RemoveVSMB failed: %s", err)
		}
	}
}

// TODO: VSMB for mapped directories
