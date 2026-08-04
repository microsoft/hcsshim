// Tool to merge Windows and Linux rootfs.tar(.gz) and delta.tar (or other files) into
// a unified rootfs (gzipped) TAR.

package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Microsoft/hcsshim/internal/ot"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TODO: add tests for:
// - general functionality (Windows + Linux)
// - adding `./` prefix
// - adding `/` suffix
// - overriding UID and GUID
// TODO: output CPIO archive?

func initOtelTracer() (func(context.Context) error, error) {
	exporter := &ot.LogrusExporter{}
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	if traceProvider == nil {
		return nil, errors.New("failed to construct OpenTelemetry tracer provider")
	}
	otel.SetTracerProvider(traceProvider)
	if otel.GetTracerProvider() != traceProvider {
		return nil, errors.New("failed to register OpenTelemetry tracer provider globally")
	}
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return traceProvider.Shutdown, nil
}

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})

	// Register our Otel logrus exporter
	_, err := initOtelTracer()
	if err != nil {
		logrus.Fatalf("failed to initialize ot tracer: %v", err)
	}

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
