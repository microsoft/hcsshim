package main

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/urfave/cli"
)

var stacksCommand = cli.Command{
	Name:      "stacks",
	Usage:     "Dump the shim and guest's goroutine stacks",
	ArgsUsage: "<shim name>",
	Before:    appargs.Validate(appargs.String),
	Action: func(c *cli.Context) error {
		shim, err := shimdiag.GetShim(c.Args()[0])
		if err != nil {
			return err
		}
		svc := shimdiag.NewShimDiagClient(shim)
		resp, err := svc.DiagStacks(context.Background(), &shimdiag.StacksRequest{})
		if err != nil {
			return err
		}

		fmt.Println("Stacks:\n", resp.Stacks)

		if resp.GuestStacks != "" {
			fmt.Println("Guest Stacks:\n", resp.GuestStacks)
		}
		return nil
	},
}
