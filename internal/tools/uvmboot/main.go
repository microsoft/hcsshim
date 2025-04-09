//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/winapi"
)

const (
	cpusArgName                 = "cpus"
	memoryArgName               = "memory"
	allowOvercommitArgName      = "allow-overcommit"
	enableDeferredCommitArgName = "enable-deferred-commit"
	measureArgName              = "measure"
	parallelArgName             = "parallel"
	countArgName                = "count"

	execCommandLineArgName = "exec"
	uvmConsolePipe         = "\\\\.\\pipe\\uvmpipe"
)

var (
	debug  bool
	useGCS bool
)

type uvmRunFunc func(string) error

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
			Name:        "debug",
			Usage:       "Enable debug information",
			Destination: &debug,
		},
		cli.BoolFlag{
			Name:        "gcs",
			Usage:       "Launch the GCS and perform requested operations via its RPC interface",
			Destination: &useGCS,
		},
	}

	app.Commands = []cli.Command{
		lcowCommand,
		wcowCommand,
		cwcowCommand,
	}

	app.Before = func(c *cli.Context) error {
		if !winapi.IsElevated() {
			log.Fatal(c.App.Name + " must be run in an elevated context")
		}

		if debug {
			logrus.SetLevel(logrus.DebugLevel)
		} else {
			logrus.SetLevel(logrus.WarnLevel)
		}

		return nil
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatalf("%v\n", err)
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
	if c.GlobalIsSet(enableDeferredCommitArgName) {
		options.EnableDeferredCommit = c.GlobalBool(enableDeferredCommitArgName)
	}
	// Always set the console pipe in uvmboot, it helps with testing/debugging
	options.ConsolePipe = uvmConsolePipe
}

// todo: add a context here to propagate cancel/timeouts to runFunc uvm

func runMany(c *cli.Context, runFunc uvmRunFunc) {
	parallelCount := c.GlobalInt(parallelArgName)

	var wg sync.WaitGroup
	wg.Add(parallelCount)
	workChan := make(chan int)
	for i := 0; i < parallelCount; i++ {
		go func() {
			for i := range workChan {
				id := fmt.Sprintf("uvmboot-%d", i)
				if err := runFunc(id); err != nil {
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

func unrecognizedError(name, value string) error {
	return fmt.Errorf("unrecognized value '%s' for option %s", name, value)
}
