// +build functional,uvmvsmb
// To run: go test -v -tags "functional uvmvsmb"

package uvm

//import (
//	"os"
//	"testing"

//	"github.com/Microsoft/hcsshim/internal/schema2"
//	"github.com/Microsoft/hcsshim/internal/test/framework"
//)

//// TestVSMB tests adding/removing VSMB layers from a v2 Windows utility VM
//func TestVSMB(t *testing.T) {
//	imageName := "microsoft/nanoserver"
//	uvm, uvmScratchDir := createWCOWUVM(t, framework.LayerFolders(t, imageName), "", nil)
//	defer os.RemoveAll(uvmScratchDir)
//	defer uvm.Terminate()

//	dir := framework.CreateTempDir(t)
//	defer os.RemoveAll(dir)
//	var iterations uint32 = 64
//	for i := 0; i < int(iterations); i++ {
//		if err := uvm.AddVSMB(dir, "", schema2.VsmbFlagReadOnly|schema2.VsmbFlagPseudoOplocks|schema2.VsmbFlagTakeBackupPrivilege|schema2.VsmbFlagCacheIO|schema2.VsmbFlagShareRead); err != nil {
//			t.Fatalf("AddVSMB failed: %s", err)
//		}
//	}
//	if len(uvm.vsmbShares) != 1 {
//		t.Fatalf("Should only be one VSMB entry")
//	}
//	if uvm.vsmbShares[dir].refCount != iterations {
//		t.Fatalf("iteration mismatch: %d %d %+v", iterations, uvm.vsmbShares[dir].refCount, uvm.vsmbShares[dir])
//	}

//	// Verify the counter matches the internal data-structure
//	c, err := uvm.GetVSMBCounter(dir)
//	if err != nil {
//		t.Fatalf("failed to find counter")
//	}
//	if uvm.vsmbShares[dir].idCounter != c {
//		t.Fatalf("counter from GetVSMBCounter doesn't match")
//	}

//	// Remove them all
//	for i := 0; i < int(iterations); i++ {
//		if err := uvm.RemoveVSMB(dir); err != nil {
//			t.Fatalf("RemoveVSMB failed: %s", err)
//		}
//	}
//	if len(uvm.vsmbShares) != 0 {
//		t.Fatalf("Should not be any vsmb entries remaining")
//	}

//}

//// TODO: VSMB for mapped directories
