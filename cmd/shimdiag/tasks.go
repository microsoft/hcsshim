//go:build windows

package main

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/urfave/cli"
)

var tasksCommand = cli.Command{
	Name:      "tasks",
	Usage:     "Dump the shims current tasks",
	ArgsUsage: "[flags] <shim name>",
	Before:    appargs.Validate(appargs.String),
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "execs",
			Usage: "Shows the execs in each task",
		},
	},
	Action: func(c *cli.Context) error {
		shim, err := shimdiag.GetShim(c.Args()[0])
		if err != nil {
			return err
		}
		svc := shimdiag.NewShimDiagClient(shim)
		resp, err := svc.DiagTasks(context.Background(), &shimdiag.TasksRequest{Execs: c.Bool("execs")})
		if err != nil {
			return err
		}

		for _, task := range resp.Tasks {
			fmt.Println(task.ID)
			if len(task.Execs) != 0 {
				fmt.Printf("|\n|----> ")
				for _, exec := range task.Execs {
					fmt.Printf("\t%s: %s\n", exec.State, exec.ID)
				}
				fmt.Printf("\n")
			}
		}

		return nil
	},
}
