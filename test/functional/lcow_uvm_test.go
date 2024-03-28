//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"fmt"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"

	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

// TestLCOWUVM_KernelArgs starts an LCOW utility VM and validates the kernel args contain the expected parameters.
func TestLCOWUVM_KernelArgs(t *testing.T) {
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
func TestLCOWUVM_Boot(t *testing.T) {
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
