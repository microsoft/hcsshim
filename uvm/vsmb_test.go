// +build functional,vsmb

// To run: go test -v -tags "functional vsmb"

package uvm

import (
	"os"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/schema2"
)

// TestVSMBRO tests adding/removing VSMB Read-Only layers from a v2 Windows utility VM
func TestVSMBRO(t *testing.T) {
	uvm, uvmScratchDir := createWCOWUVM(t, layersNanoserver, "", nil)
	defer os.RemoveAll(uvmScratchDir)
	defer uvm.Terminate()

	dir := strings.ToUpper(createTempDir(t)) // Force upper-case
	var iterations uint32 = 64
	for i := 0; i < int(iterations); i++ {
		if err := uvm.AddVSMB(dir, "", schema2.VsmbFlagReadOnly|schema2.VsmbFlagPseudoOplocks|schema2.VsmbFlagTakeBackupPrivilege|schema2.VsmbFlagCacheIO|schema2.VsmbFlagShareRead); err != nil {
			t.Fatalf("AddVSMB failed: %s", err)
		}
	}
	if len(uvm.vsmbShares) != 1 {
		t.Fatalf("Should only be one VSMB entry")
	}
	if _, ok := uvm.vsmbShares[dir]; ok {
		t.Fatalf("should not found as upper case")
	}
	if _, ok := uvm.vsmbShares[strings.ToLower(dir)]; !ok {
		t.Fatalf("not found!")
	}
	if uvm.vsmbShares[strings.ToLower(dir)].refCount != iterations {
		t.Fatalf("iteration mismatch: %d %d", iterations, uvm.vsmbShares[strings.ToLower(dir)].refCount)
	}

	// Verify the GUID matches the internal data-structure
	g, err := uvm.GetVSMBGUID(dir)
	if err != nil {
		t.Fatalf("failed to find guid")
	}
	if uvm.vsmbShares[strings.ToLower(dir)].guid != g {
		t.Fatalf("guid from GetVSMBShareGUID doesn't match")
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
