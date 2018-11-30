package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/internal/uvm"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	log "github.com/sirupsen/logrus"
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
)

func main() {
	app := cli.NewApp()
	app.Name = "uvmboot"
	app.Usage = "Boot a utility VM"

	app.Flags = []cli.Flag{
		cli.Uint64Flag{
			Name:  cpusArgName,
			Value: 2,
			Usage: "Number of CPUs on the UVM",
		},
		cli.UintFlag{
			Name:  memoryArgName,
			Value: 1024,
			Usage: "Amount of memory on the UVM, in MB",
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
		cli.BoolTFlag{
			Name:  allowOvercommitArgName,
			Usage: "Allow memory overcommit on the UVM",
		},
		cli.BoolFlag{
			Name:  enableDeferredCommitArgName,
			Usage: "Enable deferred commit on the UVM",
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
					Value: 0,
					Usage: "0 to boot from initrd, 1 to boot from VHD",
				},
				cli.UintFlag{
					Name:  vpMemMaxCountArgName,
					Value: 64,
					Usage: "Number of VPMem devices on the UVM",
				},
				cli.Uint64Flag{
					Name:  vpMemMaxSizeArgName,
					Value: 4 * 1024,
					Usage: "Size of each VPMem device, in MB",
				},
				cli.BoolFlag{
					Name:  kernelDirectArgName,
					Usage: "Use kernel direct booting for UVM",
				},
			},
			Action: func(c *cli.Context) error {
				rootFSType := uvm.PreferredRootFSType(c.Int(rootFSTypeArgName))
				vpMemMaxCount := uint32(c.Uint(vpMemMaxCountArgName))
				vpMemMaxSize := c.Uint64(vpMemMaxSizeArgName) * 1024 * 1024 // convert from MB to bytes
				cpus := c.GlobalUint64(cpusArgName)
				memory := c.GlobalUint64(memoryArgName) * 1024 * 1024 // convert from MB to bytes
				allowOvercommit := c.GlobalBoolT(allowOvercommitArgName)
				enableDeferredCommit := c.GlobalBool(enableDeferredCommitArgName)

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
								ID: id,
								Resources: &specs.WindowsResources{
									CPU: &specs.WindowsCPUResources{
										Count: &cpus,
									},
									Memory: &specs.WindowsMemoryResources{
										Limit: &memory,
									},
								},
								AllowOvercommit:      &allowOvercommit,
								EnableDeferredCommit: &enableDeferredCommit,
							},
							KernelDirect:        c.Bool(kernelDirectArgName),
							KernelBootOptions:   c.String(kernelArgsArgName),
							PreferredRootFSType: &rootFSType,
							VPMemDeviceCount:    &vpMemMaxCount,
							VPMemSizeBytes:      &vpMemMaxSize,
							SuppressGcsLogs:     c.GlobalBool(suppressOutputArgName),
						}

						// log.Infof("[%d] Starting", id)

						if err := run(&options); err != nil {
							log.Errorf("[%s] %s", id, err)
						}

						// log.Infof("[%d] Finished", id)
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
		log.Fatal(err)
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
