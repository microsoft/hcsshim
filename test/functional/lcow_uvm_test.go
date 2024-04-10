//go:build windows && functional
// +build windows,functional

package functional

import (
	"bytes"
	"context"
	"errors"
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
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
	"github.com/Microsoft/hcsshim/osversion"

	testutilities "github.com/Microsoft/hcsshim/test/internal"
	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

// test if closing a waiting (but not starting) uVM succeeds.
func TestLCOW_UVMCreateClose(t *testing.T) {
	requireFeatures(t, featureLCOW, featureUVM)
	require.Build(t, osversion.RS5)

	ctx := util.Context(context.Background(), t)
	vm, cleanup := testuvm.CreateLCOW(ctx, t, defaultLCOWOptions(ctx, t))

	testuvm.Close(ctx, t, vm)

	// also run cleanup to make sure that works fine too
	cleanup(ctx)
}

// test if waiting after creating (but not starting) an LCOW uVM returns.
func TestLCOW_UVMCreateWait(t *testing.T) {
	requireFeatures(t, featureLCOW, featureUVM)
	require.Build(t, osversion.RS5)

	ctx := util.Context(context.Background(), t)
	vm, cleanup := testuvm.CreateLCOW(ctx, t, defaultLCOWOptions(ctx, t))
	t.Cleanup(func() { cleanup(ctx) })

	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	t.Cleanup(cancel)
	switch err := vm.WaitCtx(timeoutCtx); {
	case err == nil:
		t.Fatal("wait did not error")
	case !errors.Is(err, context.DeadlineExceeded):
		t.Fatalf("wait should have errored with '%v'; got '%v'", context.DeadlineExceeded, err)
	}
}

// TestLCOWUVM_KernelArgs starts an LCOW utility VM and validates the kernel args contain the expected parameters.
func TestLCOW_UVM_KernelArgs(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureUVM)

	// TODO:
	// - opts.VPCIEnabled and `pci=off`
	// - opts.ProcessDumpLocation and `-core-dump-location`
	// - opts.ConsolePipe/opts.EnableGraphicsConsole and `console=`

	ctx := util.Context(context.Background(), t)
	numCPU := int32(2)

	for _, tc := range []struct {
		name         string
		optsFn       func(*uvm.OptionsLCOW)
		wantArgs     []string
		notWantArgs  []string
		wantDmesg    []string
		notWantDmesg []string
	}{
		//
		// initrd test cases
		//
		// Don't test initrd with SCSI or vPMEM, since boot won't use either and the settings
		// won't appear in kernel args or dmesg.
		// Kernel command line only contains `initrd=/initrd.img` if KernelDirect is disabled, which
		// implies booting from a compressed kernel.

		{
			name: "initrd kernel",
			optsFn: func(opts *uvm.OptionsLCOW) {
				opts.SCSIControllerCount = 0
				opts.VPMemDeviceCount = 0

				opts.PreferredRootFSType = uvm.PreferredRootFSTypeInitRd
				opts.RootFSFile = uvm.InitrdFile

				opts.KernelDirect = false
				opts.KernelFile = uvm.KernelFile
			},
			wantArgs: []string{fmt.Sprintf(`initrd=/%s`, uvm.InitrdFile),
				`8250_core.nr_uarts=0`, fmt.Sprintf(`nr_cpus=%d`, numCPU), `panic=-1`, `quiet`, `pci=off`},
			notWantArgs: []string{`root=`, `rootwait`, `init=`, `/dev/pmem`, `/dev/sda`, `console=`},
			wantDmesg:   []string{`initrd`, `initramfs`},
		},
		{
			name: "initrd vmlinux",
			optsFn: func(opts *uvm.OptionsLCOW) {
				opts.SCSIControllerCount = 0
				opts.VPMemDeviceCount = 0

				opts.PreferredRootFSType = uvm.PreferredRootFSTypeInitRd
				opts.RootFSFile = uvm.InitrdFile

				opts.KernelDirect = true
				opts.KernelFile = uvm.UncompressedKernelFile
			},
			wantArgs:    []string{`8250_core.nr_uarts=0`, fmt.Sprintf(`nr_cpus=%d`, numCPU), `panic=-1`, `quiet`, `pci=off`},
			notWantArgs: []string{`root=`, `rootwait`, `init=`, `/dev/pmem`, `/dev/sda`, `console=`},
			wantDmesg:   []string{`initrd`, `initramfs`},
		},

		//
		// VHD rootfs test cases
		//

		{
			name: "no SCSI single vPMEM VHD kernel",
			optsFn: func(opts *uvm.OptionsLCOW) {
				opts.SCSIControllerCount = 0
				opts.VPMemDeviceCount = 1

				opts.PreferredRootFSType = uvm.PreferredRootFSTypeVHD
				opts.RootFSFile = uvm.VhdFile

				opts.KernelDirect = false
				opts.KernelFile = uvm.KernelFile
			},
			wantArgs: []string{`root=/dev/pmem0`, `rootwait`, `init=/init`,
				`8250_core.nr_uarts=0`, fmt.Sprintf(`nr_cpus=%d`, numCPU), `panic=-1`, `quiet`, `pci=off`},
			notWantArgs:  []string{`initrd=`, `/dev/sda`, `console=`},
			notWantDmesg: []string{`initrd`, `initramfs`},
		},
		{
			name: "SCSI no vPMEM VHD kernel",
			optsFn: func(opts *uvm.OptionsLCOW) {
				opts.SCSIControllerCount = 1
				opts.VPMemDeviceCount = 0

				opts.PreferredRootFSType = uvm.PreferredRootFSTypeVHD
				opts.RootFSFile = uvm.VhdFile

				opts.KernelDirect = false
				opts.KernelFile = uvm.KernelFile
			},
			wantArgs: []string{`root=/dev/sda`, `rootwait`, `init=/init`,
				`8250_core.nr_uarts=0`, fmt.Sprintf(`nr_cpus=%d`, numCPU), `panic=-1`, `quiet`, `pci=off`},
			notWantArgs:  []string{`initrd=`, `/dev/pmem`, `console=`},
			notWantDmesg: []string{`initrd`, `initramfs`},
		},
		{
			name: "no SCSI single vPMEM VHD vmlinux",
			optsFn: func(opts *uvm.OptionsLCOW) {
				opts.SCSIControllerCount = 0
				opts.VPMemDeviceCount = 1

				opts.PreferredRootFSType = uvm.PreferredRootFSTypeVHD
				opts.RootFSFile = uvm.VhdFile

				opts.KernelDirect = true
				opts.KernelFile = uvm.UncompressedKernelFile
			},
			wantArgs: []string{`root=/dev/pmem0`, `rootwait`, `init=/init`,
				`8250_core.nr_uarts=0`, fmt.Sprintf(`nr_cpus=%d`, numCPU), `panic=-1`, `quiet`, `pci=off`},
			notWantArgs:  []string{`initrd=`, `/dev/sda`, `console=`},
			notWantDmesg: []string{`initrd`, `initramfs`},
		},
		{
			name: "SCSI no vPMEM VHD vmlinux",
			optsFn: func(opts *uvm.OptionsLCOW) {
				opts.SCSIControllerCount = 1
				opts.VPMemDeviceCount = 0

				opts.PreferredRootFSType = uvm.PreferredRootFSTypeVHD
				opts.RootFSFile = uvm.VhdFile

				opts.KernelDirect = true
				opts.KernelFile = uvm.UncompressedKernelFile
			},
			wantArgs: []string{`root=/dev/sda`, `rootwait`, `init=/init`,
				`8250_core.nr_uarts=0`, fmt.Sprintf(`nr_cpus=%d`, numCPU), `panic=-1`, `quiet`, `pci=off`},
			notWantArgs:  []string{`initrd=`, `/dev/pmem`, `console=`},
			notWantDmesg: []string{`initrd`, `initramfs`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := defaultLCOWOptions(ctx, t)
			opts.ProcessorCount = numCPU
			tc.optsFn(opts)

			if opts.KernelDirect {
				require.Build(t, 18286)
			}

			vm := testuvm.CreateAndStartLCOWFromOpts(ctx, t, opts)

			// validate the kernel args were constructed as expected
			ioArgs := testcmd.NewBufferedIO()
			cmdArgs := testcmd.Create(ctx, t, vm, &specs.Process{Args: []string{"cat", "/proc/cmdline"}}, ioArgs)
			testcmd.Start(ctx, t, cmdArgs)
			testcmd.WaitExitCode(ctx, t, cmdArgs, 0)

			ioArgs.TestStdOutContains(t, tc.wantArgs, tc.notWantArgs)

			// some boot options (notably using initrd) need to validated by looking at dmesg logs
			// dmesg will output the kernel command line as
			//
			// 	[    0.000000] Command line: <...>
			//
			// but its easier/safer to read the args directly from /proc/cmdline

			ioDmesg := testcmd.NewBufferedIO()
			cmdDmesg := testcmd.Create(ctx, t, vm, &specs.Process{Args: []string{"dmesg"}}, ioDmesg)
			testcmd.Start(ctx, t, cmdDmesg)
			testcmd.WaitExitCode(ctx, t, cmdDmesg, 0)

			ioDmesg.TestStdOutContains(t, tc.wantDmesg, tc.notWantDmesg)
		})
	}
}

// TestLCOWUVM_Boot starts and terminates a utility VM  multiple times using different boot options.
func TestLCOW_UVM_Boot(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureUVM)

	numIters := 3
	ctx := util.Context(context.Background(), t)

	for _, tc := range []struct {
		name   string
		optsFn func(*uvm.OptionsLCOW)
	}{
		{
			name: "vPMEM no kernel direct initrd",
			optsFn: func(opts *uvm.OptionsLCOW) {
				opts.KernelDirect = false
				opts.KernelFile = uvm.KernelFile

				opts.RootFSFile = uvm.InitrdFile
				opts.PreferredRootFSType = uvm.PreferredRootFSTypeInitRd

				opts.VPMemDeviceCount = 32
			},
		},
		{
			name: "vPMEM kernel direct initrd",
			optsFn: func(opts *uvm.OptionsLCOW) {
				opts.KernelDirect = true
				opts.KernelFile = uvm.UncompressedKernelFile

				opts.RootFSFile = uvm.InitrdFile
				opts.PreferredRootFSType = uvm.PreferredRootFSTypeInitRd

				opts.VPMemDeviceCount = 32
			},
		},
		{
			name: "vPMEM no kernel direct VHD",
			optsFn: func(opts *uvm.OptionsLCOW) {
				opts.KernelDirect = false
				opts.KernelFile = uvm.KernelFile

				opts.RootFSFile = uvm.VhdFile
				opts.PreferredRootFSType = uvm.PreferredRootFSTypeVHD

				opts.VPMemDeviceCount = 32
			},
		},
		{
			name: "vPMEM kernel direct VHD",
			optsFn: func(opts *uvm.OptionsLCOW) {
				opts.KernelDirect = true
				opts.KernelFile = uvm.UncompressedKernelFile

				opts.PreferredRootFSType = uvm.PreferredRootFSTypeVHD
				opts.RootFSFile = uvm.VhdFile

				opts.VPMemDeviceCount = 32
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < numIters; i++ {
				// create new options every time, in case they are modified during uVM creation
				opts := defaultLCOWOptions(ctx, t)
				tc.optsFn(opts)

				// should probably short circuit earlied, but this will skip all subsequent iterations, which works
				if opts.KernelDirect {
					require.Build(t, 18286)
				}

				vm := testuvm.CreateAndStartLCOWFromOpts(ctx, t, opts)
				testuvm.Close(ctx, t, vm)
			}
		})
	}
}

func TestLCOWSimplePodScenario(t *testing.T) {
	t.Skip("Doesn't work quite yet")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureUVM, featureContainer)

	layers := linuxImageLayers(context.Background(), t)

	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "cache.vhdx")

	// This is what gets mounted for UVM scratch
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

	// Populate the cache and generate the scratch file
	if err := lcow.CreateScratch(context.Background(), lcowUVM, uvmScratchFile, lcow.DefaultScratchSizeGB, cacheFile); err != nil {
		t.Fatal(err)
	}

	_, err := lcowUVM.SCSIManager.AddVirtualDisk(context.Background(), uvmScratchFile, false, lcowUVM.ID(), &scsi.MountConfig{})
	if err != nil {
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
	c1hcsSystem, c1Resources, err := hcsoci.CreateContainer(context.Background(), c1Opts)
	if err != nil {
		t.Fatal(err)
	}
	c2hcsSystem, c2Resources, err := hcsoci.CreateContainer(context.Background(), c2Opts)
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
