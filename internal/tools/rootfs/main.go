// Tool to merge Windows and Linux rootfs.tar(.gz) and delta.tar (or other files) into
// a unified rootfs (gzipped) TAR.

package main

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/trace"

	"github.com/Microsoft/hcsshim/internal/oc"
)

// TODO: tests ...............

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})

	trace.ApplyConfig(trace.Config{DefaultSampler: oc.DefaultSampler})
	trace.RegisterExporter(&oc.LogrusExporter{})

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	args := os.Args

	mergeCommand, err := newMergeCommand()
	if err != nil {
		return fmt.Errorf("could not create merge command: %w", err)
	}

	app := &cli.App{
		Name:  "rootfs",
		Usage: "manipulate rootfs tar(.gz) files",

		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-level",
				Aliases: []string{"lvl"},
				Usage:   "logging `level`",
				Value:   logrus.StandardLogger().Level.String(),
				Action: func(_ *cli.Context, s string) error {
					lvl, err := logrus.ParseLevel(s)
					if err == nil {
						logrus.SetLevel(lvl)
					}
					return err
				},
			},
		},

		Commands: []*cli.Command{
			mergeCommand,
		},
		DefaultCommand: mergeCommand.Name,
		ExitErrHandler: func(ctx *cli.Context, err error) {
			if err != nil {
				logrus.WithFields(logrus.Fields{
					logrus.ErrorKey: err,
					"command":       fmt.Sprintf("%#+v", args),
				}).Error(ctx.App.Name + " failed")
			}
		},
	}

	return app.Run(args)
}

// TODO: output CPIO archive

// see 	"github.com/u-root/mkuimage/cpio".fs_windows.go
type tarRecorder struct {
	inumber uint64
}

func newTarRecorder() *tarRecorder { return &tarRecorder{inumber: 2} }

func (r *tarRecorder) inode() uint64 {
	r.inumber++
	return r.inumber - 1
}
