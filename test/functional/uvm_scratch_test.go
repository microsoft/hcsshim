//go:build windows && (functional || uvmscratch)
// +build windows
// +build functional uvmscratch

package functional

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/internal/lcow"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	tuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func TestScratchCreateLCOW(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureScratch)

	tempDir := t.TempDir()
	firstUVM := tuvm.CreateAndStartLCOW(context.Background(), t, "TestCreateLCOWScratch")
	defer firstUVM.Close()

	cacheFile := filepath.Join(tempDir, "cache.vhdx")
	destOne := filepath.Join(tempDir, "destone.vhdx")
	destTwo := filepath.Join(tempDir, "desttwo.vhdx")

	if err := lcow.CreateScratch(context.Background(), firstUVM, destOne, lcow.DefaultScratchSizeGB, cacheFile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destOne); err != nil {
		t.Fatalf("destone wasn't created!")
	}
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("cacheFile wasn't created!")
	}

	targetUVM := tuvm.CreateAndStartLCOW(context.Background(), t, "TestCreateLCOWScratch_target")
	defer targetUVM.Close()

	// A non-cached create
	if err := lcow.CreateScratch(context.Background(), firstUVM, destTwo, lcow.DefaultScratchSizeGB, cacheFile); err != nil {
		t.Fatal(err)
	}

	// Make sure it can be added (verifies it has access correctly)
	scsiMount, err := targetUVM.SCSIManager.AddVirtualDisk(context.Background(), destTwo, false, targetUVM.ID(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if scsiMount.Controller() != 0 && scsiMount.LUN() != 0 {
		t.Fatal(err)
	}
	// TODO Could consider giving it a host path and verifying it's contents somehow
}

// TODO This is old test which should go here.
//// createLCOWTempDirWithSandbox uses an LCOW utility VM to create a blank
//// VHDX and format it ext4.
//func TestCreateLCOWScratch(t *testing.T) {
//	t.Skip("for now")
//	cacheDir := createTempDir(t)
//	cacheFile := filepath.Join(cacheDir, "cache.vhdx")
//	uvm, err := CreateContainer(&CreateOptions{Spec: getDefaultLinuxSpec(t)})
//	if err != nil {
//		t.Fatalf("Failed create: %s", err)
//	}
//	defer uvm.Terminate()
//	if err := uvm.Start(); err != nil {
//		t.Fatalf("Failed to start service container: %s", err)
//	}

//	// 1: Default size, cache doesn't exist, but no UVM passed. Cannot be created
//	err = CreateLCOWScratch(nil, filepath.Join(cacheDir, "default.vhdx"), lcow.DefaultScratchSizeGB, cacheFile)
//	if err == nil {
//		t.Fatalf("expected an error creating LCOW scratch")
//	}
//	if err.Error() != "cannot create scratch disk as cache is not present and no utility VM supplied" {
//		t.Fatalf("Not expecting error %s", err)
//	}

//	// 2: Default size, no cache supplied and no UVM
//	err = CreateLCOWScratch(nil, filepath.Join(cacheDir, "default.vhdx"), lcow.DefaultScratchSizeGB, "")
//	if err == nil {
//		t.Fatalf("expected an error creating LCOW scratch")
//	}
//	if err.Error() != "cannot create scratch disk as cache is not present and no utility VM supplied" {
//		t.Fatalf("Not expecting error %s", err)
//	}

//	// 3: Default size. This should work and the cache should be created.
//	err = CreateLCOWScratch(uvm, filepath.Join(cacheDir, "default.vhdx"), lcow.DefaultScratchSizeGB, cacheFile)
//	if err != nil {
//		t.Fatalf("should succeed creating default size cache file: %s", err)
//	}
//	if _, err = os.Stat(cacheFile); err != nil {
//		t.Fatalf("failed to stat cache file after created: %s", err)
//	}
//	if _, err = os.Stat(filepath.Join(cacheDir, "default.vhdx")); err != nil {
//		t.Fatalf("failed to stat default.vhdx after created: %s", err)
//	}

//	// 4: Non-defaultsize. This should work and the cache should be created.
//	err = CreateLCOWScratch(uvm, filepath.Join(cacheDir, "nondefault.vhdx"), lcow.DefaultScratchSizeGB+1, cacheFile)
//	if err != nil {
//		t.Fatalf("should succeed creating default size cache file: %s", err)
//	}
//	if _, err = os.Stat(cacheFile); err != nil {
//		t.Fatalf("failed to stat cache file after created: %s", err)
//	}
//	if _, err = os.Stat(filepath.Join(cacheDir, "nondefault.vhdx")); err != nil {
//		t.Fatalf("failed to stat default.vhdx after created: %s", err)
//	}

//}
