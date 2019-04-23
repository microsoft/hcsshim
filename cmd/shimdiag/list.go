package main

import (
	"fmt"

	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/urfave/cli"
)

var listCommand = cli.Command{
	Name:      "list",
	Usage:     "Lists running shims",
	ArgsUsage: " ",
	Before:    appargs.Validate(),
	Action: func(ctx *cli.Context) error {
		shims, err := findShims("")
		if err != nil {
			return err
		}
		for _, shim := range shims {
			fmt.Println(shim)
		}
		return nil
	},
}
