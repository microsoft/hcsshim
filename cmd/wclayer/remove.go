package main

import (
	"path/filepath"

	winio "github.com/microsoft/go-winio"
	"github.com/microsoft/hcsshim"
	"github.com/microsoft/hcsshim/internal/appargs"

	"github.com/urfave/cli"
)

var removeCommand = cli.Command{
	Name:      "remove",
	Usage:     "permanently removes a layer directory in its entirety",
	ArgsUsage: "<layer path>",
	Before:    appargs.Validate(appargs.NonEmptyString),
	Action: func(context *cli.Context) (err error) {
		path, err := filepath.Abs(context.Args().First())
		if err != nil {
			return err
		}

		err = winio.EnableProcessPrivileges([]string{winio.SeBackupPrivilege, winio.SeRestorePrivilege})
		if err != nil {
			return err
		}

		return hcsshim.DestroyLayer(driverInfo, path)
	},
}
