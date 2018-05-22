// +build functional,sandbox

// To run: go test -v -tags "functional sandbox"

package uvm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateLCOWSandbox(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	uvm := createLCOWUVM(t, "TestCreateLCOWSandbox")
	defer uvm.Terminate()

	cacheFile := filepath.Join(tempDir, "cache.vhdx")
	destOne := filepath.Join(tempDir, "destone.vhdx")
	destTwo := filepath.Join(tempDir, "desttwo.vhdx")

	if err := uvm.CreateLCOWSandbox(destOne, DefaultLCOWSandboxSizeGB, cacheFile, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destOne); err != nil {
		t.Fatalf("destone wasn't created!")
	}
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("cacheFile wasn't created!")
	}

	targetUVM := createLCOWUVM(t, "TestCreateLCOWSandbox_target")
	defer targetUVM.Terminate()

	// A non-cached create
	if err := uvm.CreateLCOWSandbox(destTwo, DefaultLCOWSandboxSizeGB, cacheFile, targetUVM.id); err != nil {
		t.Fatal(err)
	}

	// Make sure it can be added (verifies it has access correctly)
	c, l, err := targetUVM.AddSCSI(destTwo, "")
	if err != nil {
		t.Fatal(err)
	}
	if c != 0 && l != 0 {
		t.Fatal(err)
	}
	// TODO Could consider giving it a host path and verifying it's contents somehow
}
