//go:build windows

// containerd-shim-lcow-v2 is a containerd shim implementation for Linux Containers on Windows (LCOW).
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	_ "github.com/Microsoft/hcsshim/cmd/containerd-shim-lcow-v2/service/plugin"
	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/shim"

	"github.com/containerd/errdefs"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

const (
	// name is the name of lcow shim implementation.
	name = "containerd-shim-lcow-v2"
)

// Add a manifest to get proper Windows version detection.
//go:generate go tool github.com/josephspurrier/goversioninfo/cmd/goversioninfo -platform-specific

func main() {
	logrus.AddHook(log.NewHook())

	// Register our OpenCensus logrus exporter so that trace spans are emitted via logrus.
	trace.ApplyConfig(trace.Config{DefaultSampler: oc.DefaultSampler})
	trace.RegisterExporter(&oc.LogrusExporter{})

	logrus.SetFormatter(log.NopFormatter{})
	logrus.SetOutput(io.Discard)

	// Set the log configuration.
	// If we encounter an error, we exit with non-zero code.
	if err := setLogConfiguration(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s: %s", name, err)
		os.Exit(1)
	}

	// Start the shim manager event loop. The manager is responsible for
	// handling containerd start/stop lifecycle calls for the shim process.
	shim.Run(context.Background(), newShimManager(name), func(c *shim.Config) {
		// We don't want the shim package to set up logging options.
		c.NoSetupLogger = true
	})
}

// setLogConfiguration reads the runtime options from stdin and sets the log configuration.
// We only set up the log configuration for serve action.
func setLogConfiguration() error {
	// We set up the log configuration in the serve action only.
	// This is because we want to avoid reading the stdin in start action,
	// so that we can pass it along to the invocation for serve action.
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "serve" {
		// The serve process is started with stderr pointing to panic.log file.
		// We want to keep that file only for pure Go panics. Any explicit writes
		// to os.Stderr should go to stdout instead, which is connected to the parent's
		// stderr for regular logging.
		// We can safely redirect os.Stderr to os.Stdout because in case of panics,
		// the Go runtime will write the panic stack trace directly to the file descriptor,
		// bypassing os.Stderr, so it will still go to panic.log.
		os.Stderr = os.Stdout

		opts, err := shim.ReadRuntimeOptions[*runhcsopts.Options](os.Stdin)
		if err != nil {
			if !errors.Is(err, errdefs.ErrNotFound) {
				return fmt.Errorf("failed to read runtime options from stdin: %w", err)
			}
		}

		if opts != nil {
			if opts.LogLevel != "" {
				// If log level is specified, set the corresponding logrus logging level.
				lvl, err := logrus.ParseLevel(opts.LogLevel)
				if err != nil {
					return fmt.Errorf("failed to parse shim log level %q: %w", opts.LogLevel, err)
				}
				logrus.SetLevel(lvl)
			}

			if opts.ScrubLogs {
				log.SetScrubbing(true)
			}
		}
		_ = os.Stdin.Close()
	}
	return nil
}
