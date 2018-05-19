package uvm

//import (
//	"os"
//	"strings"
//	"testing"

//	"github.com/Microsoft/hcsshim/internal/schema2"
//)

//// TestWCOWVSMB tests adding/removing VSMB from a v2 Windows utility VM
//func TestWCOWVSMB(t *testing.T) {
//	t.Skip("for now")
//	v2uvm, v2uvmScratchDir := createv2WCOWUVM(t, layersNanoserver, "", nil)
//	defer os.RemoveAll(v2uvmScratchDir)
//	defer v2uvm.Terminate()

//	dir := strings.ToUpper(createTempDir(t)) // Force upper-case
//	var iterations uint32 = 64
//	for i := 0; i < int(iterations); i++ {
//		if err := v2uvm.AddVSMB(dir, hcsschemav2.VsmbFlagReadOnly|hcsschemav2.VsmbFlagPseudoOplocks|hcsschemav2.VsmbFlagTakeBackupPrivilege|hcsschemav2.VsmbFlagCacheIO|hcsschemav2.VsmbFlagShareRead); err != nil {
//			t.Fatalf("AddVSMB failed: %s", err)
//		}
//	}
//	if len(v2uvm.vsmbShares.shares) != 1 {
//		t.Fatalf("Should only be one VSMB entry")
//	}
//	if _, ok := v2uvm.vsmbShares.shares[dir]; ok {
//		t.Fatalf("should not found as upper case")
//	}
//	if _, ok := v2uvm.vsmbShares.shares[strings.ToLower(dir)]; !ok {
//		t.Fatalf("not found!")
//	}
//	if v2uvm.vsmbShares.shares[strings.ToLower(dir)].refCount != iterations {
//		t.Fatalf("iteration mismatch: %d %d", iterations, v2uvm.vsmbShares.shares[strings.ToLower(dir)].refCount)
//	}

//	// Verify the GUID matches the internal data-structure
//	g, err := v2uvm.GetVSMBGUID(dir)
//	if err != nil {
//		t.Fatalf("failed to find guid")
//	}
//	if v2uvm.vsmbShares.shares[strings.ToLower(dir)].guid != g {
//		t.Fatalf("guid from GetVSMBShareGUID doesn't match")
//	}

//	// Remove them all
//	for i := 0; i < int(iterations); i++ {
//		if err := v2uvm.RemoveVSMB(dir); err != nil {
//			t.Fatalf("RemoveVSMB failed: %s", err)
//		}
//	}
//	if len(v2uvm.vsmbShares.shares) != 0 {
//		t.Fatalf("Should not be any vsmb entries remaining")
//	}

//}
