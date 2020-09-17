package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const (
	cpusArgName                 = "cpus"
	memoryArgName               = "memory"
	allowOvercommitArgName      = "allow-overcommit"
	enableDeferredCommitArgName = "enable-deferred-commit"
	measureArgName              = "measure"
	parallelArgName             = "parallel"
	countArgName                = "count"
	debugArgName                = "debug"
	gcsArgName                  = "gcs"
	externalBridgeArgName       = "external-bridge"

	execCommandLineArgName = "exec"
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
		cli.BoolFlag{
			Name:  gcsArgName,
			Usage: "Launch the GCS and perform requested operations via its RPC interface",
		},
		cli.BoolFlag{
			Name:  externalBridgeArgName,
			Usage: "Use the external implementation of the guest connection",
		},
	}

	app.Commands = []cli.Command{
		lcowCommand,
		wcowCommand,
	}

	app.Before = func(c *cli.Context) error {
		if c.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		} else {
			logrus.SetLevel(logrus.WarnLevel)
		}
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		logrus.Fatal(err)
	}
}

func setGlobalOptions(c *cli.Context, options *uvm.Options) {
	if c.GlobalIsSet(cpusArgName) {
		options.ProcessorCount = int32(c.GlobalUint64(cpusArgName))
	}
	if c.GlobalIsSet(memoryArgName) {
		options.MemorySizeInMB = c.GlobalUint64(memoryArgName)
	}
	if c.GlobalIsSet(allowOvercommitArgName) {
		options.AllowOvercommit = c.GlobalBool(allowOvercommitArgName)
	}
	if c.GlobalIsSet(enableDeferredCommitArgName) {
		options.EnableDeferredCommit = c.GlobalBool(enableDeferredCommitArgName)
	}
	if c.GlobalIsSet(externalBridgeArgName) {
		options.ExternalGuestConnection = c.GlobalBool(externalBridgeArgName)
	}
}

func runMany(c *cli.Context, runFunc func(id string) error) {
	parallelCount := c.GlobalInt(parallelArgName)

	var wg sync.WaitGroup
	wg.Add(parallelCount)
	workChan := make(chan int)
	for i := 0; i < parallelCount; i++ {
		go func() {
			for i := range workChan {
				id := fmt.Sprintf("uvmboot-%d", i)
				err := runFunc(id)
				if err != nil {
					logrus.WithField("uvm-id", id).WithError(err).Error("failed to run UVM")
				}
			}
			wg.Done()
		}()
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
}
