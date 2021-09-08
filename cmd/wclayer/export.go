package main

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/Microsoft/hcsshim/pkg/ociwclayer"
	"github.com/urfave/cli"
)

var exportCommand = cli.Command{
	Name:  "export",
	Usage: "exports a layer to a tar file",
	Flags: []cli.Flag{
		cli.StringSliceFlag{
			Name:  "layer, l",
			Usage: "paths to the read-only parent layers",
		},
		cli.StringFlag{
			Name:  "output, o",
			Usage: "output layer tar (defaults to stdout)",
		},
		cli.BoolFlag{
			Name:  "gzip, z",
			Usage: "compress output with gzip compression",
		},
	},
	ArgsUsage: "<layer path>",
	Before:    appargs.Validate(appargs.NonEmptyString),
	Action: func(cliContext *cli.Context) (err error) {
		path, err := filepath.Abs(cliContext.Args().First())
		if err != nil {
			return err
		}

		layers, err := normalizeLayers(cliContext.StringSlice("layer"), true)
		if err != nil {
			return err
		}

		err = winio.EnableProcessPrivileges([]string{winio.SeBackupPrivilege})
		if err != nil {
			return err
		}

		fp := cliContext.String("output")
		f := os.Stdout
		if fp != "" {
			f, err = os.Create(fp)
			if err != nil {
				return err
			}
			defer f.Close()
		}
		w := io.Writer(f)
		if cliContext.Bool("gzip") {
			w = gzip.NewWriter(w)
		}

		return ociwclayer.ExportLayerToTar(context.Background(), w, path, layers)
	},
}
