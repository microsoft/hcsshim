//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"github.com/containerd/console"
	"github.com/urfave/cli"
)

const (
	bootFilesPathArgName          = "boot-files-path"
	consolePipeArgName            = "console-pipe"
	kernelDirectArgName           = "kernel-direct"
	kernelFileArgName             = "kernel-file"
	forwardStdoutArgName          = "fwd-stdout"
	forwardStderrArgName          = "fwd-stderr"
	outputHandlingArgName         = "output-handling"
	kernelArgsArgName             = "kernel-args"
	rootFSTypeArgName             = "root-fs-type"
	vpMemMaxCountArgName          = "vpmem-max-count"
	vpMemMaxSizeArgName           = "vpmem-max-size"
	scsiMountsArgName             = "mount-scsi"
	vpmemMountsArgName            = "mount-vpmem"
	shareFilesArgName             = "share"
	securityPolicyArgName         = "security-policy"
	securityHardwareFlag          = "security-hardware"
	securityPolicyEnforcerArgName = "security-policy-enforcer"
)

var (
	lcowUseTerminal     bool
	lcowDisableTimeSync bool
)

var lcowCommand = cli.Command{
	Name:  "lcow",
	Usage: "Boot an LCOW UVM",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  kernelArgsArgName,
			Value: "",
			Usage: "Additional arguments to pass to the kernel",
		},
		cli.StringFlag{
			Name:  rootFSTypeArgName,
			Usage: "Either 'initrd', 'vhd' or 'none'. (default: 'vhd' if rootfs.vhd exists)",
		},
		cli.StringFlag{
			Name:  bootFilesPathArgName,
			Usage: "The `path` to the boot files directory",
		},
		cli.UintFlag{
			Name:  vpMemMaxCountArgName,
			Usage: "Number of VPMem devices on the UVM. Uses hcsshim default if not specified",
		},
		cli.Uint64Flag{
			Name:  vpMemMaxSizeArgName,
			Usage: "Size of each VPMem device, in MB. Uses hcsshim default if not specified",
		},
		cli.BoolFlag{
			Name:  kernelDirectArgName,
			Usage: "Use kernel direct booting for UVM (default: true on builds >= 18286)",
		},
		cli.StringFlag{
			Name:  kernelFileArgName,
			Usage: "The kernel `file` to use; either 'kernel' or 'vmlinux'. (default: 'kernel')",
		},
		cli.BoolFlag{
			Name:        "disable-time-sync",
			Usage:       "Disable the time synchronization service",
			Destination: &lcowDisableTimeSync,
		},
		cli.StringFlag{
			Name:  securityPolicyArgName,
			Usage: "Security policy to set on the UVM. Leave empty to use an open door policy",
		},
		cli.StringFlag{
			Name: securityPolicyEnforcerArgName,
			Usage: "Security policy enforcer to use for a given security policy. " +
				"Leave empty to use the default enforcer",
		},
		cli.BoolFlag{
			Name:  securityHardwareFlag,
			Usage: "Use VMGS file to run on secure hardware. ('root-fs-type' must be set to 'none')",
		},
		cli.StringFlag{
			Name:  execCommandLineArgName,
			Usage: "Command to execute in the UVM.",
		},
		cli.BoolFlag{
			Name:  forwardStdoutArgName,
			Usage: "Whether stdout from the process in the UVM should be forwarded",
		},
		cli.BoolFlag{
			Name:  forwardStderrArgName,
			Usage: "Whether stderr from the process in the UVM should be forwarded",
		},
		cli.StringFlag{
			Name:  outputHandlingArgName,
			Usage: "Controls how output from UVM is handled. Use 'stdout' to print all output to stdout",
		},
		cli.StringFlag{
			Name:  consolePipeArgName,
			Usage: "Named pipe for serial console output (which will be enabled)",
		},
		cli.BoolFlag{
			Name:        "tty,t",
			Usage:       "create the process in the UVM with a TTY enabled",
			Destination: &lcowUseTerminal,
		},
		cli.StringSliceFlag{
			Name: scsiMountsArgName,
			Usage: "List of VHDs to SCSI mount into the UVM. Use repeat instances to add multiple. " +
				"Value is of the form `'host[,guest[,w]]'`, where 'host' is path to the VHD, " +
				`'guest' is an optional mount path inside the UVM, and 'w' mounts the VHD as writeable`,
		},
		cli.StringSliceFlag{
			Name: shareFilesArgName,
			Usage: "List of paths or files to plan9 share into the UVM. Use repeat instances to add multiple. " +
				"Value is of the form `'host,guest[,w]' where 'host' is path to the VHD, " +
				`'guest' is the mount path inside the UVM, and 'w' sets the shared files to writeable`,
		},
		cli.StringSliceFlag{
			Name:  vpmemMountsArgName,
			Usage: "List of VHDs to VPMem mount into the UVM. Use repeat instances to add multiple. ",
		},
	},
	Action: func(c *cli.Context) error {
		runMany(c, func(id string) error {
			ctx := context.Background()

			options, err := createLCOWOptions(ctx, c, id)
			if err != nil {
				return err
			}

			if err := runLCOW(ctx, options, c); err != nil {
				return err
			}

			return nil
		})

		return nil
	},
}

func init() {
	lcowCommand.CustomHelpTemplate = cli.CommandHelpTemplate + "EXAMPLES:\n" +
		`.\uvmboot.exe -gcs lcow -boot-files-path "C:\ContainerPlat\LinuxBootFiles" -root-fs-type vhd -t -exec "/bin/bash"`
}

func createLCOWOptions(_ context.Context, c *cli.Context, id string) (*uvm.OptionsLCOW, error) {
	options := uvm.NewDefaultOptionsLCOW(id, "")
	setGlobalOptions(c, options.Options)

	// boot
	if c.IsSet(bootFilesPathArgName) {
		options.BootFilesPath = c.String(bootFilesPathArgName)
	}

	// kernel
	if c.IsSet(kernelDirectArgName) {
		options.KernelDirect = c.Bool(kernelDirectArgName)
	}
	if c.IsSet(kernelFileArgName) {
		switch strings.ToLower(c.String(kernelFileArgName)) {
		case uvm.KernelFile:
			options.KernelFile = uvm.KernelFile
		case uvm.UncompressedKernelFile:
			options.KernelFile = uvm.UncompressedKernelFile
		default:
			return nil, unrecognizedError(c.String(kernelFileArgName), kernelFileArgName)
		}
	}
	if c.IsSet(kernelArgsArgName) {
		options.KernelBootOptions = c.String(kernelArgsArgName)
	}

	// rootfs
	if c.IsSet(rootFSTypeArgName) {
		switch strings.ToLower(c.String(rootFSTypeArgName)) {
		case "initrd":
			options.RootFSFile = uvm.InitrdFile
			options.PreferredRootFSType = uvm.PreferredRootFSTypeInitRd
		case "vhd":
			options.RootFSFile = uvm.VhdFile
			options.PreferredRootFSType = uvm.PreferredRootFSTypeVHD
		case "none":
			options.RootFSFile = ""
			options.PreferredRootFSType = uvm.PreferredRootFSTypeNA
		default:
			return nil, unrecognizedError(c.String(rootFSTypeArgName), rootFSTypeArgName)
		}
	}

	if c.IsSet(vpMemMaxCountArgName) {
		options.VPMemDeviceCount = uint32(c.Uint(vpMemMaxCountArgName))
	}
	if c.IsSet(vpMemMaxSizeArgName) {
		options.VPMemSizeBytes = c.Uint64(vpMemMaxSizeArgName) * memory.MiB // convert from MB to bytes
	}

	// GCS
	options.UseGuestConnection = useGCS
	if !useGCS {
		if c.IsSet(execCommandLineArgName) {
			options.ExecCommandLine = c.String(execCommandLineArgName)
		}
		if c.IsSet(forwardStdoutArgName) {
			options.ForwardStdout = c.Bool(forwardStdoutArgName)
		}
		if c.IsSet(forwardStderrArgName) {
			options.ForwardStderr = c.Bool(forwardStderrArgName)
		}
		if c.IsSet(outputHandlingArgName) {
			switch strings.ToLower(c.String(outputHandlingArgName)) {
			case "stdout":
				options.OutputHandlerCreator = func(*uvm.Options) uvm.OutputHandler {
					return func(r io.Reader) {
						_, _ = io.Copy(os.Stdout, r)
					}
				}
			default:
				return nil, unrecognizedError(c.String(outputHandlingArgName), outputHandlingArgName)
			}
		}
	}
	if c.IsSet(consolePipeArgName) {
		options.ConsolePipe = c.String(consolePipeArgName)
	}

	// general settings
	if lcowDisableTimeSync {
		options.DisableTimeSyncService = true
	}

	// default to open door security policy to allow resource modifications in
	// non-snp uvmboot scenarios.
	openPolicy, err := securitypolicy.NewOpenDoorPolicy().EncodeToString()
	if err != nil {
		return nil, fmt.Errorf("failed to encode open door policy: %s", err)
	}
	options.SecurityPolicy = openPolicy
	if c.IsSet(securityPolicyArgName) {
		options.SecurityPolicy = c.String(securityPolicyArgName)
	}
	if c.IsSet(securityPolicyEnforcerArgName) {
		options.SecurityPolicyEnforcer = c.String(securityPolicyEnforcerArgName)
	}
	if c.IsSet(securityHardwareFlag) {
		options.GuestStateFile = uvm.GuestStateFile
		options.SecurityPolicyEnabled = true
		options.AllowOvercommit = false
	}

	return options, nil
}

func runLCOW(ctx context.Context, options *uvm.OptionsLCOW, c *cli.Context) error {
	vm, err := uvm.CreateLCOW(ctx, options)
	if err != nil {
		return err
	}
	defer vm.Close()

	if err := vm.Start(ctx); err != nil {
		return err
	}

	if err := mountSCSI(ctx, c, vm); err != nil {
		return err
	}

	if err := shareFiles(ctx, c, vm); err != nil {
		return err
	}

	if err := mountVPMem(ctx, c, vm); err != nil {
		return err
	}

	if options.UseGuestConnection {
		if err := execViaGcs(vm, c); err != nil {
			return err
		}
		_ = vm.Terminate(ctx)
		_ = vm.Wait()

		return vm.ExitError()
	}

	return vm.Wait()
}

func execViaGcs(vm *uvm.UtilityVM, c *cli.Context) error {
	cmd := cmd.Command(vm, "/bin/sh", "-c", c.String(execCommandLineArgName))
	cmd.Log = log.L.Dup()
	if lcowUseTerminal {
		cmd.Spec.Terminal = true
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		con, err := console.ConsoleFromFile(os.Stdin)
		if err == nil {
			err = con.SetRaw()
			if err != nil {
				return err
			}
			defer func() {
				_ = con.Reset()
			}()
		}
	} else if c.String(outputHandlingArgName) == "stdout" {
		if c.Bool(forwardStdoutArgName) {
			cmd.Stdout = os.Stdout
		}
		if c.Bool(forwardStderrArgName) {
			cmd.Stderr = os.Stdout // match non-GCS behavior and forward to stdout
		}
	}

	return cmd.Run()
}
