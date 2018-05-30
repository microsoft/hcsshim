// +build functional lcow

package functional

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/functional/utilities"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/lcow"
	"github.com/Microsoft/hcsshim/internal/osversion"
	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/uvm"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// TestLCOWUVMNoSCSISingleVPMem starts an LCOW utility VM without a SCSI controller and
// only a single VPMem device. Uses initRD
func TestLCOWUVMNoSCSISingleVPMem(t *testing.T) {
	opts := &uvm.UVMOptions{
		OperatingSystem:  "linux",
		ID:               "uvm",
		VPMemDeviceCount: 1,
		NoSCSI:           true,
	}
	lcowUVM, err := uvm.Create(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := lcowUVM.Start(); err != nil {
		t.Fatal(err)
	}
	defer lcowUVM.Terminate()
	out, err := exec.Command(`hcsdiag`, `exec`, `-uvm`, lcowUVM.ID(), `dmesg`).Output()
	if err != nil {
		t.Fatal(string(err.(*exec.ExitError).Stderr))
	}
	t.Log(string(out))
}

// TestLCOWUVMNoSCSISingleVPMemVHD starts an LCOW utility VM without a SCSI controller and
// only a single VPMem device. Uses VPMEM VHD
func TestLCOWUVMNoSCSISingleVPMemVHD(t *testing.T) {
	opts := &uvm.UVMOptions{
		OperatingSystem:     "linux",
		ID:                  "uvm",
		VPMemDeviceCount:    1,
		NoSCSI:              true,
		PreferredRootFSType: uvm.PreferredRootFSTypeVHD,
	}
	lcowUVM, err := uvm.Create(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := lcowUVM.Start(); err != nil {
		t.Fatal(err)
	}
	defer lcowUVM.Terminate()
	out, err := exec.Command(`hcsdiag`, `exec`, `-uvm`, lcowUVM.ID(), `dmesg`).Output()
	if err != nil {
		t.Fatal(string(err.(*exec.ExitError).Stderr))
	}
	t.Log(string(out))
}

// TestLCOWTimeUVMStartVHD starts/terminates a utility VM booting from VPMem-
// attached root filesystem a number of times.
func TestLCOWTimeUVMStartVHD(t *testing.T) {
	t.Skip("Takes a while to run")
	testLCOWTimeUVMStart(t, uvm.PreferredRootFSTypeVHD)
}

// TestLCOWTimeUVMStartInitRD starts/terminates a utility VM booting from initrd-
// attached root file system a number of times.
func TestLCOWTimeUVMStartInitRD(t *testing.T) {
	t.Skip("Takes a while to run")
	testLCOWTimeUVMStart(t, uvm.PreferredRootFSTypeInitRd)
}

func testLCOWTimeUVMStart(t *testing.T, rfsType uvm.PreferredRootFSType) {
	testutilities.RequiresBuild(t, osversion.RS5)
	for i := 0; i < 10; i++ {
		opts := &uvm.UVMOptions{
			OperatingSystem:     "linux",
			ID:                  "uvm",
			VPMemDeviceCount:    32,
			PreferredRootFSType: rfsType,
		}
		lcowUVM, err := uvm.Create(opts)
		if err != nil {
			t.Fatal(err)
		}
		if err := lcowUVM.Start(); err != nil {
			t.Fatal(err)
		}
		lcowUVM.Terminate()
	}
}

func TestLCOWSimplePodScenario(t *testing.T) {
	t.Skip("Doesn't work quite yet")
	testutilities.RequiresBuild(t, osversion.RS5)
	alpineLayers := testutilities.LayerFolders(t, "alpine")

	cacheDir := testutilities.CreateTempDir(t)
	defer os.RemoveAll(cacheDir)
	cacheFile := filepath.Join(cacheDir, "cache.vhdx")

	// This is what gets mounted into /tmp/scratch
	uvmScratchDir := testutilities.CreateTempDir(t)
	defer os.RemoveAll(uvmScratchDir)
	uvmScratchFile := filepath.Join(uvmScratchDir, "uvmscratch.vhdx")

	// Scratch for the first container
	c1ScratchDir := testutilities.CreateTempDir(t)
	defer os.RemoveAll(c1ScratchDir)
	c1ScratchFile := filepath.Join(c1ScratchDir, "sandbox.vhdx")

	// Scratch for the second container
	c2ScratchDir := testutilities.CreateTempDir(t)
	defer os.RemoveAll(c2ScratchDir)
	c2ScratchFile := filepath.Join(c2ScratchDir, "sandbox.vhdx")

	opts := &uvm.UVMOptions{
		OperatingSystem: "linux",
		ID:              "uvm",
	}
	lcowUVM, err := uvm.Create(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := lcowUVM.Start(); err != nil {
		t.Fatal(err)
	}
	defer lcowUVM.Terminate()

	// Populate the cache and generate the scratch file for /tmp/scratch
	if err := lcow.CreateScratch(lcowUVM, uvmScratchFile, lcow.DefaultScratchSizeGB, cacheFile, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := lcowUVM.AddSCSI(uvmScratchFile, `/tmp/scratch`); err != nil {
		t.Fatal(err)
	}

	// Now create the first containers sandbox, populate a spec
	if err := lcow.CreateScratch(lcowUVM, c1ScratchFile, lcow.DefaultScratchSizeGB, cacheFile, ""); err != nil {
		t.Fatal(err)
	}
	c1Spec := testutilities.GetDefaultLinuxSpec(t)
	c1Folders := append(alpineLayers, c1ScratchDir)
	c1Spec.Windows.LayerFolders = c1Folders
	//c1Spec.Process.Args = []string{"echo", "hello", "lcow", "container", "one"}
	c1Spec.Process.Args = []string{"sleep", "120"}
	c1Opts := &hcsoci.CreateOptions{
		Spec:          c1Spec,
		HostingSystem: lcowUVM,
	}

	// Now create the second containers sandbox, populate a spec
	if err := lcow.CreateScratch(lcowUVM, c2ScratchFile, lcow.DefaultScratchSizeGB, cacheFile, ""); err != nil {
		t.Fatal(err)
	}
	c2Spec := testutilities.GetDefaultLinuxSpec(t)
	c2Folders := append(alpineLayers, c2ScratchDir)
	c2Spec.Windows.LayerFolders = c2Folders
	c2Spec.Process.Args = []string{"echo", "hello", "lcow", "container", "two"}
	c2Opts := &hcsoci.CreateOptions{
		Spec:          c2Spec,
		HostingSystem: lcowUVM,
	}

	// Create the two containers
	c1hcsSystem, c1Resources, err := CreateContainerTestWrapper(c1Opts)
	if err != nil {
		t.Fatal(err)
	}
	c2hcsSystem, c2Resources, err := CreateContainerTestWrapper(c2Opts)
	if err != nil {
		t.Fatal(err)
	}

	// Start them
	if err := c1hcsSystem.Start(); err != nil {
		t.Fatal(err)
	}
	if err := c2hcsSystem.Start(); err != nil {
		t.Fatal(err)
	}

	// Run the init process defined in the original spec
	runCommand(t, false, c2hcsSystem, nil, "hello lcow container one")

	time.Sleep(3 * time.Minute)

	hcsoci.ReleaseResources(c1Resources, lcowUVM, true)
	hcsoci.ReleaseResources(c2Resources, lcowUVM, true)
}

// Helper to launch a process in it. At the
// point of calling, the container must have been successfully created.
// TODO Convert to CreateProcessEx using full OCI spec.
func runCommand(t *testing.T, execProcess bool, hcssystem *hcs.System, ociProcessSpec *specs.Process, expectedOutput string) {
	if hcssystem == nil {
		t.Fatalf("hcssystem is nil")
	}
	computeSystem, err := hcs.OpenComputeSystem(hcssystem.ID())
	if err != nil {
		t.Fatal(err)
	}

	pc := &schema2.ProcessConfig{SchemaVersion: schemaversion.SchemaV20()}
	if execProcess {
		pc.OCIProcess = ociProcessSpec
	}

	p, err := computeSystem.CreateProcess(pc)

	if err != nil {
		t.Fatalf("Failed Create Process: %s", err)

	}
	defer p.Close()
	if err := p.Wait(); err != nil {
		t.Fatalf("Failed Wait Process: %s", err)
	}
	exitCode, err := p.ExitCode()
	if err != nil {
		t.Fatalf("Failed to obtain process exit code: %s", err)
	}
	fmt.Printf("ExitCode %d\n", exitCode)
	if exitCode != 0 {
		t.Fatalf("Non-zero exit code from process (%d)", exitCode)
	}
	_, o, _, err := p.Stdio()
	if err != nil {
		t.Fatalf("Failed to get Stdio handles for process: %s", err)
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(o)
	out := strings.TrimSpace(buf.String())
	fmt.Printf("Got %s\n", out)
	if out != expectedOutput {
		t.Fatalf("Failed to get %q from process: %q", expectedOutput, out)
	}
}
