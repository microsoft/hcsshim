// +build functional,vpmem

// To run: go test -v -tags "functional vpmem"

package uvm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/sirupsen/logrus"
)

// TestVPMEM tests adding/removing VPMem Read-Only layers from a v2 Linux utility VM
func TestVPMEM(t *testing.T) {
	uvmID := "TestVPMEM"
	uvm := createLCOWUVM(t, uvmID)
	defer uvm.Terminate()

	//dir := strings.ToUpper(createTempDir(t)) // Force upper-case  TODO TODO TODO
	var iterations uint32 = maxVPMEM
	// Use layer.vhd from the alpine image as something to add
	tempDir := createTempDir(t)
	if err := copyfile.CopyFile(filepath.Join(layersAlpine[0], "layer.vhd"), filepath.Join(tempDir, "layer.vhd"), true); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	if err := wclayer.GrantVmAccess(uvmID, filepath.Join(tempDir, "layer.vhd")); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < int(iterations); i++ {
		deviceNumber, uvmPath, err := uvm.AddVPMEM(filepath.Join(tempDir, "layer.vhd"), "/tmp/bar", true)
		if err != nil {
			t.Fatalf("AddVPMEM failed: %s", err)
		}
		logrus.Debugf("exposed as %s on %d", uvmPath, deviceNumber)
	}

	count := 0
	for _, vi := range uvm.vpmemDevices.vpmemInfo {
		if vi.hostPath != "" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("Should only be one VPMEM entry %d", count)
	}
	//	if _, ok := uvm.vsmbShares.vsmbInfo[dir]; ok {
	//		t.Fatalf("should not found as upper case")
	//	}
	//	if _, ok := uvm.vsmbShares.vsmbInfo[strings.ToLower(dir)]; !ok {
	//		t.Fatalf("not found!")
	//	}
	//	if uvm.vsmbShares.vsmbInfo[strings.ToLower(dir)].refCount != iterations {
	//		t.Fatalf("iteration mismatch: %d %d", iterations, uvm.vsmbShares.vsmbInfo[strings.ToLower(dir)].refCount)
	//	}

	//	// Verify the GUID matches the internal data-structure
	//	g, err := uvm.GetVSMBGUID(dir)
	//	if err != nil {
	//		t.Fatalf("failed to find guid")
	//	}
	//	if uvm.vsmbShares.vsmbInfo[strings.ToLower(dir)].guid != g {
	//		t.Fatalf("guid from GetVSMBShareGUID doesn't match")
	//	}

	// Remove them all
	for i := 0; i < int(iterations); i++ {
		if err := uvm.RemoveVPMEM(filepath.Join(tempDir, "layer.vhd")); err != nil {
			t.Fatalf("RemoveVPMEM failed: %s", err)
		}
	}

	count = 0
	for _, vi := range uvm.vpmemDevices.vpmemInfo {
		if vi.hostPath != "" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("Should only zero VPMEM entries %d", count)
	}

	//	if len(uvm.vsmbShares.vsmbInfo) != 0 {
	//		t.Fatalf("Should not be any vsmb entries remaining")
	//	}

}
