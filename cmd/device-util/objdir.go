//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"

	cli "github.com/urfave/cli/v2"

	"github.com/Microsoft/hcsshim/internal/winobjdir"
)

const objDirFlag = "dir"

var readObjDirCommand = &cli.Command{
	Name:    "obj-dir",
	Aliases: []string{"odir", "od"},
	Usage:   "Outputs contents of an NT object directory",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  objDirFlag,
			Usage: "Object `directory` to query.",
			Value: `\Global??`,
		},
	},
	Action: func(ctx *cli.Context) error {
		dir := ctx.String(objDirFlag)
		entries, err := winobjdir.EnumerateNTObjectDirectory(dir)
		if err != nil {
			return err
		}
		formatted := strings.Join(entries, ",")
		fmt.Fprintln(os.Stdout, formatted)
		return nil
	},
}
