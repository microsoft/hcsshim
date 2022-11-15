//go:build windows && functional
// +build windows,functional

package functional

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/lcow"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"

	testutilities "github.com/Microsoft/hcsshim/test/internal"
	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
	"github.com/Microsoft/hcsshim/test/internal/require"
	testuvm "github.com/Microsoft/hcsshim/test/internal/uvm"
)

// test if waiting after creating (but not starting) an LCOW uVM returns
func TestLCOW_UVMCreateWait(t *testing.T) {
	t.Skip("closing a created-but-not-started uVM hangs indefinitely")
	requireFeatures(t, featureLCOW)
	require.Build(t, osversion.RS5)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	vm := testuvm.CreateLCOW(ctx, t, defaultLCOWOptions(t))
	testuvm.Close(ctx, t, vm)
}

// TestLCOW_UVMNoSCSINoVPMemInitrd starts an LCOW utility VM without a SCSI controller and
// no VPMem device. Uses initrd.
func TestLCOW_UVMNoSCSINoVPMemInitrd(t *testing.T) {
	requireFeatures(t, featureLCOW)

	opts := defaultLCOWOptions(t)
	opts.SCSIControllerCount = 0
	opts.VPMemDeviceCount = 0
	opts.PreferredRootFSType = uvm.PreferredRootFSTypeInitRd
	opts.RootFSFile = uvm.InitrdFile
	opts.KernelDirect = false

	testLCOWUVMNoSCSISingleVPMem(t, opts, fmt.Sprintf("Command line: initrd=/%s", opts.RootFSFile))
}

// TestLCOW_UVMNoSCSISingleVPMemVHD starts an LCOW utility VM without a SCSI controller and
// only a single VPMem device. Uses VPMEM VHD
func TestLCOW_UVMNoSCSISingleVPMemVHD(t *testing.T) {
	requireFeatures(t, featureLCOW)

	opts := defaultLCOWOptions(t)
	opts.SCSIControllerCount = 0
	opts.VPMemDeviceCount = 1
	opts.PreferredRootFSType = uvm.PreferredRootFSTypeVHD
	opts.RootFSFile = uvm.VhdFile

	testLCOWUVMNoSCSISingleVPMem(t, opts, `Command line: root=/dev/pmem0`, `init=/init`)
}

func testLCOWUVMNoSCSISingleVPMem(t *testing.T, opts *uvm.OptionsLCOW, expected ...string) {
	t.Helper()
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW)
	ctx := context.Background()

	lcowUVM := testuvm.CreateAndStartLCOWFromOpts(ctx, t, opts)
	defer lcowUVM.Close()

	io := testcmd.NewBufferedIO()
	// c := cmd.Command(lcowUVM, "dmesg")
	c := testcmd.Create(ctx, t, lcowUVM, &specs.Process{Args: []string{"dmesg"}}, io)
	testcmd.Run(ctx, t, c)

	out, err := io.Output()

	if err != nil {
		t.Helper()
		t.Fatalf("uvm exec failed with: %s", err)
	}

	for _, s := range expected {
		if !strings.Contains(out, s) {
			t.Helper()
			t.Fatalf("Expected dmesg output to have %q: %s", s, out)
		}
	}
}

// TestLCOW_TimeUVMStartVHD starts/terminates a utility VM booting from VPMem-
// attached root filesystem a number of times.
func TestLCOW_TimeUVMStartVHD(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW)

	testLCOWTimeUVMStart(t, false, uvm.PreferredRootFSTypeVHD)
}

// TestLCOWUVMStart_KernelDirect_VHD starts/terminates a utility VM booting from
// VPMem- attached root filesystem a number of times starting from the Linux
// Kernel directly and skipping EFI.
func TestLCOW_UVMStart_KernelDirect_VHD(t *testing.T) {
	require.Build(t, 18286)
	requireFeatures(t, featureLCOW)

	testLCOWTimeUVMStart(t, true, uvm.PreferredRootFSTypeVHD)
}

// TestLCOWTimeUVMStartInitRD starts/terminates a utility VM booting from initrd-
// attached root file system a number of times.
func TestLCOW_TimeUVMStartInitRD(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW)

	testLCOWTimeUVMStart(t, false, uvm.PreferredRootFSTypeInitRd)
}

// TestLCOWUVMStart_KernelDirect_InitRd starts/terminates a utility VM booting
// from initrd- attached root file system a number of times starting from the
// Linux Kernel directly and skipping EFI.
func TestLCOW_UVMStart_KernelDirect_InitRd(t *testing.T) {
	require.Build(t, 18286)
	requireFeatures(t, featureLCOW)

	testLCOWTimeUVMStart(t, true, uvm.PreferredRootFSTypeInitRd)
}

func testLCOWTimeUVMStart(t *testing.T, kernelDirect bool, rfsType uvm.PreferredRootFSType) {
	t.Helper()
	requireFeatures(t, featureLCOW)

	for i := 0; i < 3; i++ {
		opts := defaultLCOWOptions(t)
		opts.KernelDirect = kernelDirect
		opts.VPMemDeviceCount = 32
		opts.PreferredRootFSType = rfsType
		switch opts.PreferredRootFSType {
		case uvm.PreferredRootFSTypeInitRd:
			opts.RootFSFile = uvm.InitrdFile
		case uvm.PreferredRootFSTypeVHD:
			opts.RootFSFile = uvm.VhdFile
		}

		lcowUVM := testuvm.CreateAndStartLCOWFromOpts(context.Background(), t, opts)
		lcowUVM.Close()
	}
}

func TestLCOWSimplePodScenario(t *testing.T) {
	t.Skip("Doesn't work quite yet")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureContainer)

	layers := linuxImageLayers(context.Background(), t)

	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "cache.vhdx")

	// This is what gets mounted into /tmp/scratch
	uvmScratchDir := t.TempDir()
	uvmScratchFile := filepath.Join(uvmScratchDir, "uvmscratch.vhdx")

	// Scratch for the first container
	c1ScratchDir := t.TempDir()
	c1ScratchFile := filepath.Join(c1ScratchDir, "sandbox.vhdx")

	// Scratch for the second container
	c2ScratchDir := t.TempDir()
	c2ScratchFile := filepath.Join(c2ScratchDir, "sandbox.vhdx")

	lcowUVM := testuvm.CreateAndStartLCOW(context.Background(), t, "uvm")
	defer lcowUVM.Close()

	// Populate the cache and generate the scratch file for /tmp/scratch
	if err := lcow.CreateScratch(context.Background(), lcowUVM, uvmScratchFile, lcow.DefaultScratchSizeGB, cacheFile); err != nil {
		t.Fatal(err)
	}

	var options []string
	if _, err := lcowUVM.AddSCSI(context.Background(), uvmScratchFile, `/tmp/scratch`, false, false, options, uvm.VMAccessTypeIndividual); err != nil {
		t.Fatal(err)
	}

	// Now create the first containers sandbox, populate a spec
	if err := lcow.CreateScratch(context.Background(), lcowUVM, c1ScratchFile, lcow.DefaultScratchSizeGB, cacheFile); err != nil {
		t.Fatal(err)
	}
	c1Spec := testutilities.GetDefaultLinuxSpec(t)
	c1Folders := append(layers, c1ScratchDir)
	c1Spec.Windows.LayerFolders = c1Folders
	c1Spec.Process.Args = []string{"echo", "hello", "lcow", "container", "one"}
	c1Opts := &hcsoci.CreateOptions{
		Spec:          c1Spec,
		HostingSystem: lcowUVM,
	}

	// Now create the second containers sandbox, populate a spec
	if err := lcow.CreateScratch(context.Background(), lcowUVM, c2ScratchFile, lcow.DefaultScratchSizeGB, cacheFile); err != nil {
		t.Fatal(err)
	}
	c2Spec := testutilities.GetDefaultLinuxSpec(t)
	c2Folders := append(layers, c2ScratchDir)
	c2Spec.Windows.LayerFolders = c2Folders
	c2Spec.Process.Args = []string{"echo", "hello", "lcow", "container", "two"}
	c2Opts := &hcsoci.CreateOptions{
		Spec:          c2Spec,
		HostingSystem: lcowUVM,
	}

	// Create the two containers
	c1hcsSystem, c1Resources, err := CreateContainerTestWrapper(context.Background(), c1Opts)
	if err != nil {
		t.Fatal(err)
	}
	c2hcsSystem, c2Resources, err := CreateContainerTestWrapper(context.Background(), c2Opts)
	if err != nil {
		t.Fatal(err)
	}

	// Start them. In the UVM, they'll be in the created state from runc's perspective after this.eg
	/// # runc list
	//ID                                     PID         STATUS      BUNDLE         CREATED                        OWNER
	//3a724c2b-f389-5c71-0555-ebc6f5379b30   138         running     /run/gcs/c/1   2018-06-04T21:23:39.1253911Z   root
	//7a8229a0-eb60-b515-55e7-d2dd63ffae75   158         created     /run/gcs/c/2   2018-06-04T21:23:39.4249048Z   root
	if err := c1hcsSystem.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer resources.ReleaseResources(context.Background(), c1Resources, lcowUVM, true) //nolint:errcheck

	if err := c2hcsSystem.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer resources.ReleaseResources(context.Background(), c2Resources, lcowUVM, true) //nolint:errcheck

	// Start the init process in each container and grab it's stdout comparing to expected
	runInitProcess(t, c1hcsSystem, "hello lcow container one")
	runInitProcess(t, c2hcsSystem, "hello lcow container two")
}

// Helper to run the init process in an LCOW container; verify it exits with exit
// code 0; verify stderr is empty; check output is as expected.
//
//nolint:unused // unused since tests are skipped
func runInitProcess(t *testing.T, s cow.Container, expected string) {
	t.Helper()
	var errB bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := &cmd.Cmd{
		Host:    s,
		Stderr:  &errB,
		Context: ctx,
	}
	outb, err := cmd.Output()
	if err != nil {
		t.Fatalf("stderr: %s", err)
	}
	out := string(outb)
	if strings.TrimSpace(out) != expected {
		t.Fatalf("got %q expecting %q", string(out), expected)
	}
}
