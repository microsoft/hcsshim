package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/urfave/cli"
)

var listCommand = cli.Command{
	Name:      "list",
	Usage:     "Lists running shims",
	ArgsUsage: "[flags]",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "pids",
			Usage: "Shows the process IDs of each shim",
		},
	},
	Before: appargs.Validate(),
	Action: func(ctx *cli.Context) error {
		pids := ctx.Bool("pids")
		shims, err := shimdiag.FindShims("")
		if err != nil {
			return err
		}

		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 0, 8, 0, '\t', 0)

		if pids {
			fmt.Fprintln(w, "Shim \t Pid")
		}

		for _, shim := range shims {
			if pids {
				pid, err := getPid(shim)
				if err != nil {
					return err
				}
				fmt.Fprintf(w, "%s \t %d\n", shim, pid)
			} else {
				fmt.Fprintln(w, shim)
			}
		}
		w.Flush()
		return nil
	},
}
