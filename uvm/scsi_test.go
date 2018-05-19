// +build functional

//
// These  tests must run on a system setup to run both Argons and Xenons,
// have docker installed, and have the nanoserver (WCOW) and alpine (LCOW)
// base images installed. The nanoserver image MUST match the build of the
// host.
//
// This also needs an RS5+ host supporting the v2 schema.
//
// We rely on docker as the tools to extract a container image aren't
// open source. We use it to find the location of the base image on disk.
//

package uvm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	///	"github.com/Microsoft/hcsshim"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

var (
	// Obtained from docker - for the base images used in the tests
	layersNanoserver []string // Nanoserver matching the build
	layersAlpine     []string
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	layersNanoserver = getLayers("microsoft/nanoserver:latest")
	layersAlpine = getLayers("alpine")
}

func getLayers(imageName string) []string {
	cmd := exec.Command("docker", "inspect", imageName, "-f", `"{{.GraphDriver.Data.dir}}"`)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		panic("failed to get layers. Is the daemon running?")
	}
	imagePath := strings.Replace(strings.TrimSpace(out.String()), `"`, ``, -1)
	layers := getLayerChain(imagePath)
	return append([]string{imagePath}, layers...)
}

func getLayerChain(layerFolder string) []string {
	jPath := filepath.Join(layerFolder, "layerchain.json")
	content, err := ioutil.ReadFile(jPath)
	if os.IsNotExist(err) {
		panic("layerchain not found")
	} else if err != nil {
		panic("failed to read layerchain")
	}

	var layerChain []string
	err = json.Unmarshal(content, &layerChain)
	if err != nil {
		panic("failed to unmarshal layerchain")
	}
	return layerChain
}

// createTempDir creates a temporary directory
func createTempDir(t *testing.T) string {
	tempDir, err := ioutil.TempDir("", "hcsshimtestcase")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %s", err)
	}
	return tempDir
}

// createWCOWTempDirWithSandbox uses HCS to create a sandbox with VM group access
// in a temporary directory. Returns the directory, the "containerID" which is
// really the foldername where the sandbox is, and a constructed DriverInfo
// structure which is required for calling v1 APIs. Strictly VM group access is
// not required for an argon.
// TODO: This is wrong anyway. Need to search the folders.
func createWCOWTempDirWithSandbox(t *testing.T) string {
	tempDir := createTempDir(t)
	//di := hcsshim.DriverInfo{HomeDir: filepath.Dir(tempDir)}
	if err := wclayer.CreateSandboxLayer(filepath.Base(tempDir), layersAlpine); err != nil {
		t.Fatalf("Failed CreateSandboxLayer: %s", err)
	}
	return tempDir
}

// createLCOWTempDirWithSandbox uses an LCOW utility VM to create a blank
// VHDX and format it ext4.
func createLCOWTempDirWithSandbox(t *testing.T) (string, string) {
	if lcowUVM == nil {
		opts := &UVMOptions{
			OperatingSystem: "linux",
			Id:              "global_service_vm_for_testing",
		}
		uvm, err := Create(opts)
		if err != nil {
			t.Fatal(err)
		}
		defer uvm.Terminate()
		if err := uvm.Start(); err != nil {
			t.Fatal(err)
		}

	}
	cacheSandboxDir := createTempDir(t)
	cacheSandboxFile := filepath.Join(cacheSandboxDir, "sandbox.vhdx")
	if err := lcowUVM.CreateLCOWScratch(cacheSandboxFile, DefaultLCOWScratchSizeGB, ""); err != nil {
		t.Fatalf("failed to create EXT4 sandbox for LCOW test cases: %s", err)
	}
	return cacheSandboxDir, filepath.Base(cacheSandboxDir)
}

// Helper to create a utility VM. Returns the UtilityVM object; folder used as its scratch
func createWCOWUVM(t *testing.T, uvmLayers []string, uvmId string, resources *specs.WindowsResources) (*UtilityVM, string) {
	scratchDir := createTempDir(t)

	opts := &UVMOptions{
		OperatingSystem: "windows",
		LayerFolders:    append(uvmLayers, scratchDir),
		Resources:       resources,
	}
	uvm, err := Create(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := uvm.Start(); err != nil {
		t.Fatal(err)
	}

	return uvm, scratchDir
}

// TestAllocateSCSI tests allocateSCSI/deallocateSCSI/findSCSIAttachment
func TestAllocateSCSI(t *testing.T) {
	t.Skip("for now")
	uvm, uvmScratchDir := createWCOWUVM(t, layersNanoserver, "", nil)
	defer os.RemoveAll(uvmScratchDir)
	defer uvm.Terminate()

	c, l, err := uvm.findSCSIAttachment(filepath.Join(uvmScratchDir, `sandbox.vhdx`))
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
	if uvm.scsiLocations.hostPath[0][0] == "" {
		t.Fatalf("0:0 should still be taken")
	}
	c, l, err = uvm.findSCSIAttachment(filepath.Join(uvmScratchDir, `sandbox.vhdx`))
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
	//t.Skip("for now")
	uvm, uvmScratchDir := createWCOWUVM(t, layersNanoserver, "", nil)
	defer os.RemoveAll(uvmScratchDir)
	defer uvm.Terminate()

	testAddRemoveSCSI(t, uvm, `c:\`, "windows")
}

// TODO: SCSI v2 LCOW?

func testAddRemoveSCSI(t *testing.T, uvm *UtilityVM, pathPrefix string, operatingSystem string) {
	numDisks := 63 // Windows: 63 as the UVM scratch is at 0:0
	if operatingSystem == "linux" {
		numDisks-- // HCS v1 for Linux has the UVM scratch at 0:0 and reserves 0:1 for the container scratch, even if it's not attached.
	}

	// Create a bunch of directories each containing sandbox.vhdx
	disks := make([]string, numDisks)
	for i := 0; i < numDisks; i++ {
		if operatingSystem == "windows" {
			disks[i] = createWCOWTempDirWithSandbox(t)
		} else {
			disks[i], _ = createLCOWTempDirWithSandbox(t)
		}
		defer os.RemoveAll(disks[i])
		disks[i] = filepath.Join(disks[i], `sandbox.vhdx`)
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

	// TODO: Could extend to validate can't add a 64th disk (windows). 63rd (linux).
}
