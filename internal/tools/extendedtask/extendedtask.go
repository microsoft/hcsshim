package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Microsoft/hcsshim/internal/extendedtask"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "extendedtask"
	app.Usage = "tool for getting compute info"
	app.Commands = []cli.Command{
		processorInfoCommand,
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var processorInfoCommand = cli.Command{
	Name:      "processorInfo",
	ArgsUsage: "<shim name> <target container ID>",
	Action: func(ctx *cli.Context) error {
		args := ctx.Args()
		var (
			shimName    = args[0]
			containerID = args[1]
		)
		shim, err := shimdiag.GetShim(shimName)
		if err != nil {
			return err
		}
		svc := extendedtask.NewExtendedTaskClient(shim)
		resp, err := svc.ComputeProcessorInfo(context.Background(), &extendedtask.ComputeProcessorInfoRequest{ID: containerID})
		if err != nil {
			return err
		}
		fmt.Println("CPU count:\n", resp.Count)
		return nil
	},
}
