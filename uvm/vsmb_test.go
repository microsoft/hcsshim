// +build functional,vsmb

// To run: go test -v -tags "functional vsmb"

package uvm

import (
	"os"
	"testing"

	"github.com/Microsoft/hcsshim/internal/schema2"
)

// TestVSMB tests adding/removing VSMB layers from a v2 Windows utility VM
func TestVSMB(t *testing.T) {
	uvm, uvmScratchDir := createWCOWUVM(t, layersNanoserver, "", nil)
	defer os.RemoveAll(uvmScratchDir)
	defer uvm.Terminate()

	dir := createTempDir(t)
	defer os.RemoveAll(dir)
	var iterations uint32 = 64
	for i := 0; i < int(iterations); i++ {
		if err := uvm.AddVSMB(dir, "", schema2.VsmbFlagReadOnly|schema2.VsmbFlagPseudoOplocks|schema2.VsmbFlagTakeBackupPrivilege|schema2.VsmbFlagCacheIO|schema2.VsmbFlagShareRead); err != nil {
			t.Fatalf("AddVSMB failed: %s", err)
		}
	}
	if len(uvm.vsmbShares) != 1 {
		t.Fatalf("Should only be one VSMB entry")
	}
	if uvm.vsmbShares[dir].refCount != iterations {
		t.Fatalf("iteration mismatch: %d %d %+v", iterations, uvm.vsmbShares[dir].refCount, uvm.vsmbShares[dir])
	}

	// Verify the counter matches the internal data-structure
	c, err := uvm.GetVSMBCounter(dir)
	if err != nil {
		t.Fatalf("failed to find counter")
	}
	if uvm.vsmbShares[dir].idCounter != c {
		t.Fatalf("counter from GetVSMBCounter doesn't match")
	}

	// Remove them all
	for i := 0; i < int(iterations); i++ {
		if err := uvm.RemoveVSMB(dir); err != nil {
			t.Fatalf("RemoveVSMB failed: %s", err)
		}
	}
	if len(uvm.vsmbShares) != 0 {
		t.Fatalf("Should not be any vsmb entries remaining")
	}

}

// TODO: VSMB for mapped directories
