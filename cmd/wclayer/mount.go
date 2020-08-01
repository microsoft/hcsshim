package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim"
	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/urfave/cli"
)

var mountCommand = cli.Command{
	Name:      "mount",
	Usage:     "activates a scratch, optionally mounted to provided target",
	ArgsUsage: "<scratch path> [target path]",
	Before:    appargs.Validate(appargs.NonEmptyString, appargs.Optional(appargs.String)),
	Flags: []cli.Flag{
		cli.StringSliceFlag{
			Name:  "layer, l",
			Usage: "paths to the parent layers for this layer",
		},
	},
	Action: func(context *cli.Context) (err error) {
		path, err := filepath.Abs(context.Args().Get(0))
		if err != nil {
			return err
		}

		targetPath, err := filepath.Abs(context.Args().Get(1))
		if err != nil {
			return err
		}

		layers, err := normalizeLayers(context.StringSlice("layer"), true)
		if err != nil {
			return err
		}

		err = hcsshim.ActivateLayer(driverInfo, path)
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				hcsshim.DeactivateLayer(driverInfo, path)
			}
		}()

		err = hcsshim.PrepareLayer(driverInfo, path, layers)
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				hcsshim.UnprepareLayer(driverInfo, path)
			}
		}()

		mountPath, err := hcsshim.GetLayerMountPath(driverInfo, path)
		if err != nil {
			return err
		}

		if context.NArg() == 2 {
			if err = setVolumeMountPoint(targetPath, mountPath); err != nil {
				return err
			}
			_, err = fmt.Println(targetPath)
			return err
		}

		_, err = fmt.Println(mountPath)
		return err
	},
}

var unmountCommand = cli.Command{
	Name:      "unmount",
	Usage:     "deactivates a scratch, optionally unmounting",
	ArgsUsage: "<scratch path> [mounted path]",
	Before:    appargs.Validate(appargs.NonEmptyString, appargs.Optional(appargs.String)),
	Action: func(context *cli.Context) (err error) {
		path, err := filepath.Abs(context.Args().Get(0))
		if err != nil {
			return err
		}

		mountedPath, err := filepath.Abs(context.Args().Get(1))
		if err != nil {
			return err
		}

		if context.NArg() == 2 {
			if err = deleteVolumeMountPoint(mountedPath); err != nil {
				return err
			}
		}

		err = hcsshim.UnprepareLayer(driverInfo, path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		err = hcsshim.DeactivateLayer(driverInfo, path)
		if err != nil {
			return err
		}
		return nil
	},
}
