//go:build windows

package main

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/containerd/console"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const (
	consolePipeArgName    = "console-pipe"
	kernelDirectArgName   = "kernel-direct"
	forwardStdoutArgName  = "fwd-stdout"
	forwardStderrArgName  = "fwd-stderr"
	outputHandlingArgName = "output-handling"
	kernelArgsArgName     = "kernel-args"
	rootFSTypeArgName     = "root-fs-type"
	vpMemMaxCountArgName  = "vpmem-max-count"
	vpMemMaxSizeArgName   = "vpmem-max-size"
)

var (
	lcowUseTerminal bool
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
			Usage: "Either 'initrd' or 'vhd'. (default: 'vhd' if rootfs.vhd exists)",
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
	},
	Action: func(c *cli.Context) error {
		runMany(c, func(id string) error {
			options := uvm.NewDefaultOptionsLCOW(id, "")
			setGlobalOptions(c, options.Options)
			useGcs := c.GlobalBool(gcsArgName)
			options.UseGuestConnection = useGcs

			if c.IsSet(kernelDirectArgName) {
				options.KernelDirect = c.Bool(kernelDirectArgName)
			}
			if c.IsSet(rootFSTypeArgName) {
				switch strings.ToLower(c.String(rootFSTypeArgName)) {
				case "initrd":
					options.RootFSFile = uvm.InitrdFile
					options.PreferredRootFSType = uvm.PreferredRootFSTypeInitRd
				case "vhd":
					options.RootFSFile = uvm.VhdFile
					options.PreferredRootFSType = uvm.PreferredRootFSTypeVHD
				default:
					logrus.Fatalf("Unrecognized value '%s' for option %s", c.String(rootFSTypeArgName), rootFSTypeArgName)
				}
			}
			if c.IsSet(kernelArgsArgName) {
				options.KernelBootOptions = c.String(kernelArgsArgName)
			}
			if c.IsSet(vpMemMaxCountArgName) {
				options.VPMemDeviceCount = uint32(c.Uint(vpMemMaxCountArgName))
			}
			if c.IsSet(vpMemMaxSizeArgName) {
				options.VPMemSizeBytes = c.Uint64(vpMemMaxSizeArgName) * memory.MiB // convert from MB to bytes
			}
			if !useGcs {
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
						options.OutputHandler = uvm.OutputHandler(func(r io.Reader) {
							_, _ = io.Copy(os.Stdout, r)
						})
					default:
						logrus.Fatalf("Unrecognized value '%s' for option %s", c.String(outputHandlingArgName), outputHandlingArgName)
					}
				}
			}
			if c.IsSet(consolePipeArgName) {
				options.ConsolePipe = c.String(consolePipeArgName)
			}

			if err := runLCOW(context.TODO(), options, c); err != nil {
				return err
			}
			return nil
		})

		return nil
	},
}

func runLCOW(ctx context.Context, options *uvm.OptionsLCOW, c *cli.Context) error {
	uvm, err := uvm.CreateLCOW(ctx, options)
	if err != nil {
		return err
	}
	defer uvm.Close()

	if err := uvm.Start(ctx); err != nil {
		return err
	}

	if options.UseGuestConnection {
		if err := execViaGcs(uvm, c); err != nil {
			return err
		}
		_ = uvm.Terminate(ctx)
		_ = uvm.Wait()
		return uvm.ExitError()
	}

	return uvm.Wait()
}

func execViaGcs(vm *uvm.UtilityVM, c *cli.Context) error {
	cmd := cmd.Command(vm, "/bin/sh", "-c", c.String(execCommandLineArgName))
	cmd.Log = logrus.NewEntry(logrus.StandardLogger())
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
