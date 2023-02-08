//go:build windows && (functional || uvmscsi)
// +build windows
// +build functional uvmscsi

package functional

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Microsoft/hcsshim/internal/wclayer"

	"github.com/Microsoft/hcsshim/internal/lcow"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/internal"
	"github.com/Microsoft/hcsshim/test/internal/require"
	tuvm "github.com/Microsoft/hcsshim/test/internal/uvm"
	"github.com/sirupsen/logrus"
)

// TestSCSIAddRemovev2LCOW validates adding and removing SCSI disks
// from a utility VM in both attach-only and with a container path.
func TestSCSIAddRemoveMultipleLCOW(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureSCSI)

	u := tuvm.CreateAndStartLCOWFromOpts(context.Background(), t, defaultLCOWOptions(t))
	defer u.Close()

	testSCSIAddRemoveMultiple(t, u, `/run/gcs/c/0/scsi`, "linux", []string{})
}

func TestSCSIAddRemoveSingleLCOW(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureSCSI)

	u := tuvm.CreateAndStartLCOWFromOpts(context.Background(), t, defaultLCOWOptions(t))
	defer u.Close()

	testSCSIAddRemoveSingle(t, u, `/run/gcs/c/0/scsi`, "linux", []string{})
}

// TestSCSIAddRemoveWCOW validates adding and removing SCSI disks
// from a utility VM in both attach-only and with a container path.
func TestSCSIAddRemoveSingleWCOW(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW, featureSCSI)

	// TODO make the image configurable to the build we're testing on
	u, layers, _ := tuvm.CreateWCOWUVM(context.Background(), t, t.Name(), "mcr.microsoft.com/windows/nanoserver:1909")
	defer u.Close()

	testSCSIAddRemoveMultiple(t, u, `c:\`, "windows", layers)
}

func TestSCSIAddRemoveMultipleWCOW(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW, featureSCSI)

	// TODO make the image configurable to the build we're testing on
	u, layers, _ := tuvm.CreateWCOWUVM(context.Background(), t, t.Name(), "mcr.microsoft.com/windows/nanoserver:1909")
	defer u.Close()

	testSCSIAddRemoveSingle(t, u, `c:\`, "windows", layers)
}

func TestSCSIWithEmptyAndNonEmptyUVMPathLCOW(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureSCSI)

	u := tuvm.CreateAndStartLCOWFromOpts(context.Background(), t, defaultLCOWOptions(t))
	defer u.Close()

	numDisks := 64
	pathPrefix := `/run/gcs/c/0/scsi`
	// Create a bunch of directories each containing sandbox.vhdx
	disks := make([]string, numDisks)
	for i := 0; i < numDisks; i++ {
		tempDir := testutilities.CreateLCOWBlankRWLayer(context.Background(), t)
		disks[i] = filepath.Join(tempDir, `sandbox.vhdx`)
	}

	logrus.Debugln("Adding scsi disks with empty uvmPaths")
	useUvmPathPrefix := false
	mounts, err := testAddSCSI(u, disks, pathPrefix, useUvmPathPrefix, false)
	if err != nil {
		t.Fatalf("failed to add SCSI device: %v", err)
	}

	// Try to re-add
	logrus.Debugln("Next - try re-adding scsi disks with non-empty uvmPaths")
	useUvmPathPrefix = true
	reMounts, err := testAddSCSI(u, disks, pathPrefix, useUvmPathPrefix, true)
	if err != nil {
		t.Fatalf("failed to add SCSI device: %v", err)
	}

	// validate that all remounted scsi devices have a non empty uvm path
	for _, m := range reMounts {
		if m.UVMPath == "" {
			t.Fatalf("expected uvmPath to be non-empty for scsi disk: %v", m.HostPath)
		}
	}

	logrus.Debugln("Next - Remove emtpy uvmPath scsi mounts")
	err = testRemoveAllSCSI(u, mounts)
	if err != nil {
		t.Fatalf("failed to remove SCSI disk: %v", err)
	}
	logrus.Debugln("Next - Remove non-emtpy uvmPath scsi mounts")
	err = testRemoveAllSCSI(u, reMounts)
	if err != nil {
		t.Fatalf("failed to remove SCSI disk: %v", err)
	}
}

func testAddSCSI(u *uvm.UtilityVM, disks []string, pathPrefix string, usePath bool, reAdd bool) ([]*uvm.SCSIMount, error) {
	mounts := []*uvm.SCSIMount{}
	for i := range disks {
		uvmPath := ""
		if usePath {
			uvmPath = fmt.Sprintf(`%s%d`, pathPrefix, i)
		}
		var options []string
		scsiMount, err := u.AddSCSI(context.Background(), disks[i], uvmPath, false, false, options, uvm.VMAccessTypeIndividual)
		if err != nil {
			return nil, err
		}
		if reAdd && scsiMount.UVMPath != uvmPath {
			return nil, fmt.Errorf("expecting existing path to be %s but it is %s", uvmPath, scsiMount.UVMPath)
		}
		mounts = append(mounts, scsiMount)
	}
	return mounts, nil
}

func testRemoveAllSCSI(u *uvm.UtilityVM, scsiMounts []*uvm.SCSIMount) error {
	for _, m := range scsiMounts {
		if err := u.RemoveSCSIMount(context.Background(), m.HostPath, m.UVMPath); err != nil {
			return err
		}
	}
	return nil
}

// TODO this test is only needed until WCOW supports adding the same scsi device to
// multiple containers
func testSCSIAddRemoveSingle(t *testing.T, u *uvm.UtilityVM, pathPrefix string, operatingSystem string, wcowImageLayerFolders []string) {
	t.Helper()
	numDisks := 63 // Windows: 63 as the UVM scratch is at 0:0
	if operatingSystem == "linux" {
		numDisks++ //
	}

	// Create a bunch of directories each containing sandbox.vhdx
	disks := make([]string, numDisks)
	for i := 0; i < numDisks; i++ {
		tempDir := ""
		if operatingSystem == "windows" {
			tempDir = testutilities.CreateWCOWBlankRWLayer(t, wcowImageLayerFolders)
		} else {
			tempDir = testutilities.CreateLCOWBlankRWLayer(context.Background(), t)
		}
		disks[i] = filepath.Join(tempDir, `sandbox.vhdx`)
	}

	// Add each of the disks to the utility VM. Attach-only, no container path
	useUvmPathPrefix := false
	logrus.Debugln("First - adding in attach-only")
	mounts, err := testAddSCSI(u, disks, pathPrefix, useUvmPathPrefix, false)
	if err != nil {
		t.Fatalf("failed to add SCSI device: %v", err)
	}

	// Remove them all
	logrus.Debugln("Removing them all")
	err = testRemoveAllSCSI(u, mounts)
	if err != nil {
		t.Fatalf("failed to remove SCSI disk: %v", err)
	}

	// Now re-add but providing a container path
	useUvmPathPrefix = true
	logrus.Debugln("Next - re-adding with a container path")
	mounts, err = testAddSCSI(u, disks, pathPrefix, useUvmPathPrefix, false)
	if err != nil {
		t.Fatalf("failed to add SCSI device: %v", err)
	}

	logrus.Debugln("Next - Removing them")
	err = testRemoveAllSCSI(u, mounts)
	if err != nil {
		t.Fatalf("failed to remove SCSI disk: %v", err)
	}

	// TODO: Could extend to validate can't add a 64th disk (windows). 65th (linux).
}

func testSCSIAddRemoveMultiple(t *testing.T, u *uvm.UtilityVM, pathPrefix string, operatingSystem string, wcowImageLayerFolders []string) {
	t.Helper()
	numDisks := 63 // Windows: 63 as the UVM scratch is at 0:0
	if operatingSystem == "linux" {
		numDisks++ //
	}

	// Create a bunch of directories each containing sandbox.vhdx
	disks := make([]string, numDisks)
	for i := 0; i < numDisks; i++ {
		tempDir := ""
		if operatingSystem == "windows" {
			tempDir = testutilities.CreateWCOWBlankRWLayer(t, wcowImageLayerFolders)
		} else {
			tempDir = testutilities.CreateLCOWBlankRWLayer(context.Background(), t)
		}
		disks[i] = filepath.Join(tempDir, `sandbox.vhdx`)
	}

	// Add each of the disks to the utility VM. Attach-only, no container path
	useUvmPathPrefix := false
	logrus.Debugln("First - adding in attach-only")
	mounts, err := testAddSCSI(u, disks, pathPrefix, useUvmPathPrefix, false)
	if err != nil {
		t.Fatalf("failed to add SCSI device: %v", err)
	}

	// Try to re-add.
	// We only support re-adding the same scsi device for lcow right now
	logrus.Debugln("Next - trying to re-add")
	reMounts, err := testAddSCSI(u, disks, pathPrefix, useUvmPathPrefix, true)
	if err != nil {
		t.Fatalf("failed to re-add SCSI device: %v", err)
	}

	// Remove them all
	logrus.Debugln("Removing them all")
	// first removal decrements ref count
	err = testRemoveAllSCSI(u, mounts)
	if err != nil {
		t.Fatalf("failed to remove SCSI disk: %v", err)
	}
	// second removal actually removes the device
	err = testRemoveAllSCSI(u, reMounts)
	if err != nil {
		t.Fatalf("failed to remove SCSI disk: %v", err)
	}

	// Now re-add but providing a container path
	logrus.Debugln("Next - re-adding with a container path")
	useUvmPathPrefix = true
	mounts, err = testAddSCSI(u, disks, pathPrefix, useUvmPathPrefix, false)
	if err != nil {
		t.Fatalf("failed to add SCSI device: %v", err)
	}

	// Try to re-add
	logrus.Debugln("Next - trying to re-add")
	reMounts, err = testAddSCSI(u, disks, pathPrefix, useUvmPathPrefix, true)
	if err != nil {
		t.Fatalf("failed to add SCSI device: %v", err)
	}

	logrus.Debugln("Next - Removing them")
	// first removal decrements ref count
	err = testRemoveAllSCSI(u, mounts)
	if err != nil {
		t.Fatalf("failed to remove SCSI disk: %v", err)
	}
	// second removal actually removes the device
	err = testRemoveAllSCSI(u, reMounts)
	if err != nil {
		t.Fatalf("failed to remove SCSI disk: %v", err)
	}

	// TODO: Could extend to validate can't add a 64th disk (windows). 65th (linux).
}

func TestParallelScsiOps(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureSCSI)

	u := tuvm.CreateAndStartLCOWFromOpts(context.Background(), t, defaultLCOWOptions(t))
	defer u.Close()

	// Create a sandbox to use
	tempDir := t.TempDir()
	if err := lcow.CreateScratch(context.Background(), u, filepath.Join(tempDir, "sandbox.vhdx"), lcow.DefaultScratchSizeGB, ""); err != nil {
		t.Fatalf("failed to create EXT4 scratch for LCOW test cases: %s", err)
	}
	copySandbox := func(dir string, workerId, iteration int) (string, error) {
		orig, err := os.Open(filepath.Join(dir, "sandbox.vhdx"))
		if err != nil {
			return "", err
		}
		defer orig.Close()
		path := filepath.Join(dir, fmt.Sprintf("%d-%d-sandbox.vhdx", workerId, iteration))
		new, err := os.Create(path)
		if err != nil {
			return "", err
		}
		defer new.Close()

		_, err = io.Copy(new, orig)
		if err != nil {
			return "", err
		}
		return path, nil
	}

	// Note: maxWorkers cannot be > 64 for this code to work
	maxWorkers := 16
	opsChan := make(chan int, maxWorkers)
	opsWg := sync.WaitGroup{}
	opsWg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go func(scsiIndex int) {
			for {
				iteration, ok := <-opsChan
				if !ok {
					break
				}
				// Copy the goal sandbox.vhdx to a new path so we don't get the cached location
				path, err := copySandbox(tempDir, scsiIndex, iteration)
				if err != nil {
					t.Errorf("failed to copy sandbox for worker: %d, iteration: %d with err: %v", scsiIndex, iteration, err)
					continue
				}
				err = wclayer.GrantVmAccess(context.Background(), u.ID(), path)
				if err != nil {
					os.Remove(path)
					t.Errorf("failed to grantvmaccess for worker: %d, iteration: %d with err: %v", scsiIndex, iteration, err)
					continue
				}

				var options []string
				scsiMount, err := u.AddSCSI(context.Background(), path, "", false, false, options, uvm.VMAccessTypeIndividual)
				if err != nil {
					os.Remove(path)
					t.Errorf("failed to AddSCSI for worker: %d, iteration: %d with err: %v", scsiIndex, iteration, err)
					continue
				}
				err = u.RemoveSCSIMount(context.Background(), path, scsiMount.UVMPath)
				if err != nil {
					t.Errorf("failed to RemoveSCSI for worker: %d, iteration: %d with err: %v", scsiIndex, iteration, err)
					// This worker cant continue because the index is dead. We have to stop
					break
				}
				scsiMount, err = u.AddSCSI(context.Background(), path, fmt.Sprintf("/run/gcs/c/0/scsi/%d", iteration), false, false, options, uvm.VMAccessTypeIndividual)
				if err != nil {
					os.Remove(path)
					t.Errorf("failed to AddSCSI for worker: %d, iteration: %d with err: %v", scsiIndex, iteration, err)
					continue
				}
				err = u.RemoveSCSIMount(context.Background(), path, scsiMount.UVMPath)
				if err != nil {
					t.Errorf("failed to RemoveSCSI for worker: %d, iteration: %d with err: %v", scsiIndex, iteration, err)
					// This worker cant continue because the index is dead. We have to stop
					break
				}
				os.Remove(path)
			}
			opsWg.Done()
		}(i)
	}

	scsiOps := 1000
	for i := 0; i < scsiOps; i++ {
		opsChan <- i
	}
	close(opsChan)

	opsWg.Wait()
}
