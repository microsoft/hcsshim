// +build functional,scsi

// To run: go test -v -tags "functional scsi"

// These  tests must run on a system setup to run both Argons and Xenons,
// have docker installed, and have the nanoserver (WCOW) and alpine (LCOW)
// base images installed. The nanoserver image MUST match the build of the
// host.
//
// This also needs an RS5+ host supporting the v2 schema.
//
// We rely on docker as the tools to extract a container image aren't
// open source. We use it to find the location of the base image on disk.

package uvm

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// TestAllocateSCSI tests allocateSCSI/deallocateSCSI/findSCSIAttachment
func TestAllocateSCSI(t *testing.T) {
	uvm, uvmScratchDir := createWCOWUVM(t, layersNanoserver, "", nil)
	defer os.RemoveAll(uvmScratchDir)
	defer uvm.Terminate()

	c, l, _, err := uvm.findSCSIAttachment(filepath.Join(uvmScratchDir, `sandbox.vhdx`))
	if err != nil {
		t.Fatalf("failed to find sandbox %s", err)
	}
	if c != 0 && l != 0 {
		t.Fatalf("sandbox at %d:%d", c, l)
	}

	for i := 0; i <= (4*64)-2; i++ { // 4 controllers, each with 64 slots but 0:0 is the UVM scratch
		controller, lun, err := uvm.allocateSCSI(`anything`)
		if err != nil {
			t.Fatalf("unexpected error %s", err)
		}
		if lun != (i+1)%64 {
			t.Fatalf("unexpected LUN:%d i=%d", lun, i)
		}
		if controller != (i+1)/64 {
			t.Fatalf("unexpected controller:%d i=%d", controller, i)
		}
	}
	_, _, err = uvm.allocateSCSI(`shouldfail`)
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != "no free SCSI locations" {
		t.Fatalf("expected to have run out of SCSI slots")
	}

	for c := 0; c < 4; c++ {
		for l := 0; l < 64; l++ {
			if !(c == 0 && l == 0) {
				uvm.deallocateSCSI(c, l)
			}
		}
	}
	if uvm.scsiLocations.scsiInfo[0][0].hostPath == "" {
		t.Fatalf("0:0 should still be taken")
	}
	c, l, _, err = uvm.findSCSIAttachment(filepath.Join(uvmScratchDir, `sandbox.vhdx`))
	if err != nil {
		t.Fatalf("failed to find sandbox %s", err)
	}
	if c != 0 && l != 0 {
		t.Fatalf("sandbox at %d:%d", c, l)
	}
}

// TestAddRemoveSCSIv2WCOW validates adding and removing SCSI disks
// from a utility VM in both attach-only and with a container path. Also does
// negative testing so that a disk can't be attached twice.
func TestAddRemoveSCSIWCOW(t *testing.T) {
	uvm, uvmScratchDir := createWCOWUVM(t, layersNanoserver, "", nil)
	defer os.RemoveAll(uvmScratchDir)
	defer uvm.Terminate()

	testAddRemoveSCSI(t, uvm, `c:\`, "windows")
}

// TestAddRemoveSCSIv2KCIW validates adding and removing SCSI disks
// from a utility VM in both attach-only and with a container path. Also does
// negative testing so that a disk can't be attached twice.
func TestAddRemoveSCSILCOW(t *testing.T) {
	uvm := createLCOWUVM(t, "TestAddRemoveSCSILCOW")
	defer uvm.Terminate()

	testAddRemoveSCSI(t, uvm, `/`, "linux")
}

func testAddRemoveSCSI(t *testing.T, uvm *UtilityVM, pathPrefix string, operatingSystem string) {
	numDisks := 63 // Windows: 63 as the UVM scratch is at 0:0
	if operatingSystem == "linux" {
		numDisks++ //
	}

	// Create a bunch of directories each containing sandbox.vhdx
	disks := make([]string, numDisks)
	for i := 0; i < numDisks; i++ {
		tempDir := ""
		if operatingSystem == "windows" {
			tempDir = createWCOWTempDirWithSandbox(t)
		} else {
			tempDir = createLCOWTempDirWithSandbox(t, uvm.id)
		}
		defer os.RemoveAll(tempDir)
		disks[i] = filepath.Join(tempDir, `sandbox.vhdx`)
	}

	// Add each of the disks to the utility VM. Attach-only, no container path
	logrus.Debugln("First - adding in attach-only")
	for i := 0; i < numDisks; i++ {
		_, _, err := uvm.AddSCSI(disks[i], "")
		if err != nil {
			t.Fatalf("failed to add scsi disk %d %s: %s", i, disks[i], err)
		}
	}

	// Try to re-add. These should all fail.
	logrus.Debugln("Next - trying to re-add")
	for i := 0; i < numDisks; i++ {
		_, _, err := uvm.AddSCSI(disks[i], "")
		if err == nil {
			t.Fatalf("should not be able to re-add the same SCSI disk!")
		}
	}

	// Remove them all
	logrus.Debugln("Removing them all")
	for i := 0; i < numDisks; i++ {
		if err := uvm.RemoveSCSI(disks[i]); err != nil {
			t.Fatalf("expected success: %s", err)
		}
	}

	// Now re-add but providing a container path
	logrus.Debugln("Next - re-adding with a container path")
	for i := 0; i < numDisks; i++ {
		_, _, err := uvm.AddSCSI(disks[i], fmt.Sprintf(`%s%d`, pathPrefix, i))
		if err != nil {
			time.Sleep(10 * time.Minute)
			t.Fatalf("failed to add scsi disk %d %s: %s", i, disks[i], err)
		}
	}

	// Try to re-add. These should all fail.
	logrus.Debugln("Next - trying to re-add")
	for i := 0; i < numDisks; i++ {
		_, _, err := uvm.AddSCSI(disks[i], fmt.Sprintf(`%s%d`, pathPrefix, i))
		if err == nil {
			t.Fatalf("should not be able to re-add the same SCSI disk!")
		}
	}

	// Remove them all
	logrus.Debugln("Next - Removing them")
	for i := 0; i < numDisks; i++ {
		if err := uvm.RemoveSCSI(disks[i]); err != nil {
			t.Fatalf("expected success: %s", err)
		}
	}

	// TODO: Could extend to validate can't add a 64th disk (windows). 65th (linux).
}
