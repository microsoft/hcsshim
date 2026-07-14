//go:build windows && wcowprocess

// containerd-shim-wcowprocess-v2 is a containerd shim for process-isolated Windows containers (WCOW).
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/cmd/containerd-shim-wcowprocess-v2/service"
	_ "github.com/Microsoft/hcsshim/cmd/containerd-shim-wcowprocess-v2/service/plugin"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/shim"
	"github.com/containerd/errdefs"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

// Add a manifest to get proper Windows version detection.
//go:generate pwsh -Command "../../scripts/New-ResourceObjectFile.ps1 -ErrorAction 'Stop' -Destination '.' -Name 'containerd-shim-wcowprocess-v2' -UseVersionFile -Architecture 'all'"

func main() {
	logrus.AddHook(log.NewHook())

	// Emit OpenCensus trace spans via logrus.
	trace.ApplyConfig(trace.Config{DefaultSampler: oc.DefaultSampler})
	trace.RegisterExporter(&oc.LogrusExporter{})

	logrus.SetFormatter(log.NopFormatter{})
	logrus.SetOutput(io.Discard)

	// Set the log configuration.
	// If we encounter an error, we exit with non-zero code.
	if err := setLogConfiguration(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s: %s", service.ShimName, err)
		os.Exit(1)
	}

	// The manager handles containerd's start/stop lifecycle for the shim process.
	shim.Run(context.Background(), newShimManager(service.ShimName), func(c *shim.Config) {
		c.NoSetupLogger = true
	})
}

// setLogConfiguration reads the runtime options from stdin and sets the log configuration.
// This is only done for the serve action so that start can forward stdin to the serve child.
func setLogConfiguration() error {
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "serve" {
		// Any explicit writes to os.Stderr should go to stdout; the Go runtime still
		// writes pure panics to the panic.log file descriptor directly.
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

			// Scrubbing is enabled by default (via init() in internal/log/scrub.go).
			// Only disable if the option is explicitly set to false.
			if opts.ScrubLogs != nil && !*opts.ScrubLogs {
				log.SetScrubbing(false)
			}
		}
		_ = os.Stdin.Close()
	}
	return nil
}
