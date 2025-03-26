//go:build windows

package main

import (
	"context"
	"os"

	"github.com/containerd/console"
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
)

var (
	cwcowBootVHD           string
	cwcowEFIVHD            string
	cwcowScratchVHD        string
	cwcowVMGSPath          string
	cwcowDisableSecureBoot bool
	cwcowIsolationMode     string
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
			Usage:       "VHD at the provided path MUST have the EFI boot partition and be properly formatted for UEFI boot.",
			Destination: &cwcowEFIVHD,
			Required:    true,
		},
		cli.StringFlag{
			Name:        "boot-cim-vhd",
			Usage:       "A VHD containing the block CIM that contains the OS files.",
			Destination: &cwcowBootVHD,
			Required:    true,
		},
		cli.StringFlag{
			Name:        "scratch-vhd",
			Usage:       "A scratch VHD for the UVM",
			Destination: &cwcowScratchVHD,
			Required:    true,
		},
		cli.StringFlag{
			Name:        vmgsFilePathArgName,
			Usage:       "VMGS file path (only applies when confidential mode is enabled). This option is only applicable in confidential mode.",
			Destination: &cwcowVMGSPath,
			Required:    true,
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
	},
	Action: func(c *cli.Context) error {
		runMany(c, func(id string) error {
			options := uvm.NewDefaultOptionsWCOW(id, "")
			options.ProcessorCount = 2
			options.MemorySizeInMB = 2048
			options.AllowOvercommit = false
			options.EnableDeferredCommit = false
			options.DumpDirectoryPath = "C:\\crashdumps"

			// confidential specific options
			options.SecurityPolicyEnabled = true
			options.DisableSecureBoot = cwcowDisableSecureBoot
			options.GuestStateFilePath = cwcowVMGSPath
			options.IsolationType = cwcowIsolationMode
			// always enable graphics console with uvmboot - helps with testing/debugging
			options.EnableGraphicsConsole = true
			options.BootFiles = &uvm.WCOWBootFiles{
				BootType: uvm.BlockCIMBoot,
				BlockCIMFiles: &uvm.BlockCIMBootFiles{
					BootCIMVHDPath: cwcowBootVHD,
					EFIVHDPath:     cwcowEFIVHD,
					ScratchVHDPath: cwcowScratchVHD,
				},
			}
			setGlobalOptions(c, options.Options)
			tempDir, err := os.MkdirTemp("", "uvmboot")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tempDir)

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
