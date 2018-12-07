package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/internal/uvm"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const (
	kernelArgsArgName           = "kernel-args"
	rootFSTypeArgName           = "root-fs-type"
	vpMemMaxCountArgName        = "vpmem-max-count"
	vpMemMaxSizeArgName         = "vpmem-max-size"
	cpusArgName                 = "cpus"
	memoryArgName               = "memory"
	allowOvercommitArgName      = "allow-overcommit"
	enableDeferredCommitArgName = "enable-deferred-commit"
	measureArgName              = "measure"
	parallelArgName             = "parallel"
	countArgName                = "count"
	kernelDirectArgName         = "kernel-direct"
	suppressOutputArgName       = "suppress-output"
	execCommandLineArgName      = "exec"
	forwardStdoutArgName        = "fwd-stdout"
	forwardStderrArgName        = "fwd-stderr"
	debugArgName                = "debug"
)

func main() {
	app := cli.NewApp()
	app.Name = "uvmboot"
	app.Usage = "Boot a utility VM"

	app.Flags = []cli.Flag{
		cli.Uint64Flag{
			Name:  cpusArgName,
			Usage: "Number of CPUs on the UVM. Uses hcsshim default if not specified",
		},
		cli.UintFlag{
			Name:  memoryArgName,
			Usage: "Amount of memory on the UVM, in MB. Uses hcsshim default if not specified",
		},
		cli.BoolFlag{
			Name:  measureArgName,
			Usage: "Measure wall clock time of the UVM run",
		},
		cli.IntFlag{
			Name:  parallelArgName,
			Value: 1,
			Usage: "Number of UVMs to boot in parallel",
		},
		cli.IntFlag{
			Name:  countArgName,
			Value: 1,
			Usage: "Total number of UVMs to run",
		},
		cli.BoolFlag{
			Name:  suppressOutputArgName,
			Usage: "Hide output from the UVM",
		},
		cli.BoolFlag{
			Name:  allowOvercommitArgName,
			Usage: "Allow memory overcommit on the UVM",
		},
		cli.BoolFlag{
			Name:  enableDeferredCommitArgName,
			Usage: "Enable deferred commit on the UVM",
		},
		cli.BoolFlag{
			Name:  debugArgName,
			Usage: "Enable debug level logging in HCSShim",
		},
	}

	app.Commands = []cli.Command{
		cli.Command{
			Name:  "lcow",
			Usage: "Boot an LCOW UVM",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  kernelArgsArgName,
					Value: "",
					Usage: "Additional arguments to pass to the kernel",
				},
				cli.UintFlag{
					Name:  rootFSTypeArgName,
					Usage: "0 to boot from initrd, 1 to boot from VHD. Uses hcsshim default if not specified",
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
					Usage: "Use kernel direct booting for UVM",
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
			},
			Action: func(c *cli.Context) error {
				if c.GlobalBool("debug") {
					logrus.SetLevel(logrus.DebugLevel)
				}

				parallelCount := c.GlobalInt(parallelArgName)

				var wg sync.WaitGroup
				wg.Add(parallelCount)

				workChan := make(chan int)

				runFunc := func(workChan <-chan int) {
					for {
						i, ok := <-workChan

						if !ok {
							wg.Done()
							return
						}

						id := fmt.Sprintf("uvmboot-%d", i)

						options := uvm.OptionsLCOW{
							Options: &uvm.Options{
								ID:        id,
								Resources: &specs.WindowsResources{},
							},
						}

						if c.GlobalIsSet(cpusArgName) {
							val := c.GlobalUint64(cpusArgName)
							options.Options.Resources.CPU = &specs.WindowsCPUResources{
								Count: &val,
							}
						}
						if c.GlobalIsSet(memoryArgName) {
							val := c.GlobalUint64(memoryArgName)
							options.Options.Resources.Memory = &specs.WindowsMemoryResources{
								Limit: &val,
							}
						}
						if c.GlobalIsSet(allowOvercommitArgName) {
							val := c.GlobalBool(allowOvercommitArgName)
							options.Options.AllowOvercommit = &val
						}
						if c.GlobalIsSet(enableDeferredCommitArgName) {
							val := c.GlobalBool(enableDeferredCommitArgName)
							options.Options.EnableDeferredCommit = &val
						}
						if c.GlobalIsSet(suppressOutputArgName) {
							options.SuppressGcsLogs = c.GlobalBool(suppressOutputArgName)
						}

						if c.IsSet(kernelDirectArgName) {
							options.KernelDirect = c.Bool(kernelDirectArgName)
						}
						if c.IsSet(rootFSTypeArgName) {
							val := uvm.PreferredRootFSType(c.Int(rootFSTypeArgName))
							options.PreferredRootFSType = &val
						}
						if c.IsSet(kernelArgsArgName) {
							options.KernelBootOptions = c.String(kernelArgsArgName)
						}
						if c.IsSet(vpMemMaxCountArgName) {
							val := uint32(c.Uint(vpMemMaxCountArgName))
							options.VPMemDeviceCount = &val
						}
						if c.IsSet(vpMemMaxSizeArgName) {
							val := c.Uint64(vpMemMaxSizeArgName) * 1024 * 1024 // convert from MB to bytes
							options.VPMemSizeBytes = &val
						}
						if c.IsSet(execCommandLineArgName) {
							options.ExecCommandLine = c.String(execCommandLineArgName)
						}
						if c.IsSet(forwardStdoutArgName) {
							val := c.Bool(forwardStdoutArgName)
							options.ForwardStdout = &val
						}
						if c.IsSet(forwardStderrArgName) {
							val := c.Bool(forwardStderrArgName)
							options.ForwardStderr = &val
						}

						if err := run(&options); err != nil {
							logrus.WithField("uvm-id", id).Error(err)
						}
					}
				}

				for i := 0; i < parallelCount; i++ {
					go runFunc(workChan)
				}

				start := time.Now()

				for i := 0; i < c.GlobalInt(countArgName); i++ {
					workChan <- i
				}

				close(workChan)

				wg.Wait()

				if c.GlobalBool(measureArgName) {
					fmt.Println("Elapsed time:", time.Since(start))
				}

				return nil
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		logrus.Fatal(err)
	}
}

func run(options *uvm.OptionsLCOW) error {
	uvm, err := uvm.CreateLCOW(options)
	if err != nil {
		return err
	}
	defer uvm.Close()

	if err := uvm.Start(); err != nil {
		return err
	}

	if err := uvm.Wait(); err != nil {
		return err
	}

	return nil
}
