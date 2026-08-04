//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containerd/console"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/uvm"
)

const (
	confidentialArgName  = "confidential"
	vmgsFilePathArgName  = "vmgs-path"
	disableSBArgName     = "disable-secure-boot"
	isolationTypeArgName = "isolation-type"
	writableEFIArgName   = "writable-efi"
	debugModeArgName     = "debug-mode"
	debugDataPathArgName = "debug-data-path"

	// default policy (that allows all operations) used when no policy is provided
	allowAllPolicy = "cGFja2FnZSBwb2xpY3kKCmFwaV92ZXJzaW9uIDo9ICIwLjExLjAiCmZyYW1ld29ya192ZXJzaW9uIDo9ICIwLjQuMCIKCm1vdW50X2NpbXMgOj0geyJhbGxvd2VkIjogdHJ1ZX0KbW91bnRfZGV2aWNlIDo9IHsiYWxsb3dlZCI6IHRydWV9Cm1vdW50X292ZXJsYXkgOj0geyJhbGxvd2VkIjogdHJ1ZX0KY3JlYXRlX2NvbnRhaW5lciA6PSB7ImFsbG93ZWQiOiB0cnVlLCAiZW52X2xpc3QiOiBudWxsLCAiYWxsb3dfc3RkaW9fYWNjZXNzIjogdHJ1ZX0KdW5tb3VudF9kZXZpY2UgOj0geyJhbGxvd2VkIjogdHJ1ZX0KdW5tb3VudF9vdmVybGF5IDo9IHsiYWxsb3dlZCI6IHRydWV9CmV4ZWNfaW5fY29udGFpbmVyIDo9IHsiYWxsb3dlZCI6IHRydWUsICJlbnZfbGlzdCI6IG51bGx9CmV4ZWNfZXh0ZXJuYWwgOj0geyJhbGxvd2VkIjogdHJ1ZSwgImVudl9saXN0IjogbnVsbCwgImFsbG93X3N0ZGlvX2FjY2VzcyI6IHRydWV9CnNodXRkb3duX2NvbnRhaW5lciA6PSB7ImFsbG93ZWQiOiB0cnVlfQpzaWduYWxfY29udGFpbmVyX3Byb2Nlc3MgOj0geyJhbGxvd2VkIjogdHJ1ZX0KcGxhbjlfbW91bnQgOj0geyJhbGxvd2VkIjogdHJ1ZX0KcGxhbjlfdW5tb3VudCA6PSB7ImFsbG93ZWQiOiB0cnVlfQpnZXRfcHJvcGVydGllcyA6PSB7ImFsbG93ZWQiOiB0cnVlfQpkdW1wX3N0YWNrcyA6PSB7ImFsbG93ZWQiOiB0cnVlfQpydW50aW1lX2xvZ2dpbmcgOj0geyJhbGxvd2VkIjogdHJ1ZX0KbG9hZF9mcmFnbWVudCA6PSB7ImFsbG93ZWQiOiB0cnVlfQpzY3JhdGNoX21vdW50IDo9IHsiYWxsb3dlZCI6IHRydWV9CnNjcmF0Y2hfdW5tb3VudCA6PSB7ImFsbG93ZWQiOiB0cnVlfQo="
)

var (
	cwcowBootVHD           string
	cwcowEFIVHD            string
	cwcowScratchVHD        string
	cwcowVMGSPath          string
	cwcowBootFilesPath     string
	cwcowDisableSecureBoot bool
	cwcowIsolationMode     string
	cwcowSecurityPolicy    string
	cwcowWritableEFI       bool
	cwcowDebugMode         bool
	cwcowDebugDataPath     string
)

var cwcowCommand = cli.Command{
	Name:  "cwcow",
	Usage: "boot a confidential WCOW UVM",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:        "exec",
			Usage:       "Command to execute in the UVM.",
			Destination: &wcowCommandLine,
		},
		cli.BoolFlag{
			Name:        "tty,t",
			Usage:       "create the process in the UVM with a TTY enabled",
			Destination: &wcowUseTerminal,
		},
		cli.StringFlag{
			Name:        "efi-vhd",
			Usage:       "VHD at the provided path MUST have the EFI boot partition and be properly formatted for UEFI boot. Ignored when --" + bootFilesPathArgName + " is provided.",
			Destination: &cwcowEFIVHD,
		},
		cli.StringFlag{
			Name:        "boot-cim-vhd",
			Usage:       "A VHD containing the block CIM that contains the OS files. Ignored when --" + bootFilesPathArgName + " is provided.",
			Destination: &cwcowBootVHD,
		},
		cli.StringFlag{
			Name:        "scratch-vhd",
			Usage:       "A scratch VHD for the UVM",
			Destination: &cwcowScratchVHD,
			Required:    true,
		},
		cli.StringFlag{
			Name:        vmgsFilePathArgName,
			Usage:       "VMGS file path (only applies when confidential mode is enabled). Ignored when --" + bootFilesPathArgName + " is provided.",
			Destination: &cwcowVMGSPath,
		},
		cli.StringFlag{
			Name:        bootFilesPathArgName,
			Usage:       "Directory containing the confidential boot files. When provided, the EFI VHD (boot.vhd), boot CIM VHD (rootfs.vhd) and VMGS file (selected from --" + isolationTypeArgName + ") are derived from it, and --efi-vhd/--boot-cim-vhd/--" + vmgsFilePathArgName + " must not be set. Provide either this or all three individual files.",
			Destination: &cwcowBootFilesPath,
		},
		cli.BoolFlag{
			Name:        disableSBArgName,
			Usage:       "Disables Secure Boot when running the UVM in confidential mode. This option is only applicable in confidential mode.",
			Destination: &cwcowDisableSecureBoot,
		},
		cli.StringFlag{
			Name:        isolationTypeArgName,
			Usage:       "VM Isolation type (one of Disabled, GuestStateOnly, VirtualizationBasedSecurity, SecureNestedPaging or TrustDomain). Applicable only when using the confidential mode. This option is only applicable in confidential mode.",
			Destination: &cwcowIsolationMode,
			Required:    true,
		},
		cli.StringFlag{
			Name:        securityPolicyArgName,
			Usage:       "Security policy that should be enforced inside the UVM. If none is provided, default policy that allows all operations will be used.",
			Destination: &cwcowSecurityPolicy,
			Value:       allowAllPolicy,
		},
		cli.BoolFlag{
			Name:        writableEFIArgName,
			Usage:       "Attaches the EFI VHD as read-write instead of read-only. This allows the UVM to modify the contents of the VHD, be careful when using this option!",
			Destination: &cwcowWritableEFI,
		},
		cli.BoolFlag{
			Name:        debugModeArgName,
			Usage:       "Enables debug mode: the boot/EFI VHD and scratch VHD are saved to the directory given by --" + debugDataPathArgName + " when the UVM is torn down. Combine with --" + writableEFIArgName + " to capture the bootstat trace.",
			Destination: &cwcowDebugMode,
		},
		cli.StringFlag{
			Name:        debugDataPathArgName,
			Usage:       "Directory to save the boot/EFI VHD and scratch VHD to when --" + debugModeArgName + " is enabled. Required when debug mode is enabled.",
			Destination: &cwcowDebugDataPath,
		},
	},
	Action: func(c *cli.Context) error {
		runMany(c, func(id string) error {
			// Resolve the boot files: either derive them from a single directory or use the
			// individually-provided VHD/VMGS paths. The scratch VHD is always provided separately.
			efiVHD, bootVHD, vmgsPath := cwcowEFIVHD, cwcowBootVHD, cwcowVMGSPath
			if cwcowBootFilesPath != "" {
				if cwcowEFIVHD != "" || cwcowBootVHD != "" || cwcowVMGSPath != "" {
					return fmt.Errorf("--%s cannot be combined with --efi-vhd, --boot-cim-vhd or --%s", bootFilesPathArgName, vmgsFilePathArgName)
				}
				root, err := filepath.Abs(cwcowBootFilesPath)
				if err != nil {
					return err
				}
				efiVHD = uvm.ConfidentialEFIPath(root)
				bootVHD = uvm.ConfidentialBootCIMPath(root)
				vmgsPath = uvm.ConfidentialVMGSPath(root, cwcowIsolationMode)
			} else if cwcowEFIVHD == "" || cwcowBootVHD == "" || cwcowVMGSPath == "" {
				return fmt.Errorf("either --%s or all of --efi-vhd, --boot-cim-vhd and --%s must be provided", bootFilesPathArgName, vmgsFilePathArgName)
			}

			bootVHD, err := filepath.Abs(bootVHD)
			if err != nil {
				return err
			}
			efiVHD, err = filepath.Abs(efiVHD)
			if err != nil {
				return err
			}
			scratchVHD, err := filepath.Abs(cwcowScratchVHD)
			if err != nil {
				return err
			}

			options := uvm.NewDefaultOptionsWCOW(id, "")
			options.ProcessorCount = 2
			options.MemorySizeInMB = 2048
			options.AllowOvercommit = false
			options.EnableDeferredCommit = false

			// confidential specific options
			options.SecurityPolicyEnabled = true
			options.SecurityPolicy = cwcowSecurityPolicy
			options.DisableSecureBoot = cwcowDisableSecureBoot
			options.GuestStateFilePath = vmgsPath
			options.IsolationType = cwcowIsolationMode

			// graphics console helps with testing/debugging however, it
			// doesn't work in SNP isolation mode.
			options.EnableGraphicsConsole = cwcowIsolationMode != "SecureNestedPaging"
			options.WritableEFI = cwcowWritableEFI

			options.DebugMode = cwcowDebugMode
			options.DebugDataPath = cwcowDebugDataPath
			if cwcowDebugMode && cwcowDebugDataPath == "" {
				return fmt.Errorf("--%s must be set to a non-empty path when --%s is enabled", debugDataPathArgName, debugModeArgName)
			}

			options.BootFiles = &uvm.WCOWBootFiles{
				BootType: uvm.BlockCIMBoot,
				BlockCIMFiles: &uvm.BlockCIMBootFiles{
					BootCIMVHDPath: bootVHD,
					EFIVHDPath:     efiVHD,
					ScratchVHDPath: scratchVHD,
				},
			}
			setGlobalOptions(c, options.Options)

			vm, err := uvm.CreateWCOW(context.TODO(), options)
			if err != nil {
				return err
			}
			defer vm.Close()
			if err := vm.Start(context.TODO()); err != nil {
				return err
			}
			if wcowCommandLine != "" {
				cmd := cmd.Command(vm, "cmd.exe", "/c", wcowCommandLine)
				cmd.Spec.User.Username = `NT AUTHORITY\SYSTEM`
				cmd.Log = log.L.Dup()
				if wcowUseTerminal {
					cmd.Spec.Terminal = true
					cmd.Stdin = os.Stdin
					cmd.Stdout = os.Stdout
					con, err := console.ConsoleFromFile(os.Stdin)
					if err == nil {
						csz, err := con.Size()
						if err != nil {
							return fmt.Errorf("failed to get console size: %w", err)
						}
						cmd.Spec.ConsoleSize = &specs.Box{
							Height: uint(csz.Height),
							Width:  uint(csz.Width),
						}
						err = con.SetRaw()
						if err != nil {
							return err
						}
						defer func() {
							_ = con.Reset()
						}()
					}
				} else {
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stdout
				}
				err = cmd.Run()
				if err != nil {
					return err
				}
			}
			_ = vm.Terminate(context.TODO())
			_ = vm.Wait()
			return vm.ExitError()
		})
		return nil
	},
}
