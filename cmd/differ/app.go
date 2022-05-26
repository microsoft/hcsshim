//go:build windows

package main

import (
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"
	exec "golang.org/x/sys/execabs"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
)

/*
todo:
restricted token
run on not default-desktop
 https://docs.microsoft.com/en-us/windows/win32/secauthz/restricted-tokens

todo:
before re-exec would need runtime parameters from payload (ie, tracing, files to allow access to)
intercept pipe and pass along command-specific payload to re-exec
do not allow access to upstream containerd pipe for payload
*/

const (
	appName           = "hcsshim-differr"
	appCapabilityName = appName + "-capability"
)

var (
	_useAppContainer bool
	_useLPAC         bool
)

var appCommands = []*cli.Command{
	decompressCommand,
	convertCommand,
	wclayerCommand,
	testCommand,
}

func app() *cli.App {
	app := &cli.App{
		Name:           appName,
		Usage:          "Containerd stream processors for applying for Windows container (WCOW and LCOW) diffs and layers",
		Commands:       appCommands,
		ExitErrHandler: errHandler,
		Before:         beforeApp,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "app-container",
				Aliases:     []string{"ac"},
				Usage:       "isolate using app containers; will use a restricted token if false",
				Destination: &_useAppContainer,
				Value:       true,
			},
			&cli.BoolFlag{
				Name:        "lpac",
				Aliases:     []string{"l"},
				Usage:       "isolate using less privileged app containers (LPAC); only valid with 'app-container' flag",
				Destination: &_useLPAC,
			},
			&cli.BoolFlag{
				Name:   reExecFlagName,
				Usage:  "set after re-execing into this command with proper permissions and environment variables",
				Hidden: true,
			},
		},
	}
	return app
}

func beforeApp(c *cli.Context) (err error) {
	if err := setupLogging(); err != nil {
		return fmt.Errorf("logging setup: %w", err)
	}
	log.G(c.Context).Info("set up logging")
	return nil
}

func errHandler(c *cli.Context, err error) {
	if err == nil {
		return
	}
	// reexec will return an exit code, so check for that edge case and
	if ee := (&exec.ExitError{}); errors.As(err, &ee) {
		err = cli.Exit("", ee.ExitCode())
	} else {
		n := c.App.Name
		if nn := c.Command.FullName(); nn != "" {
			n += " " + nn
		}
		err = cli.Exit(fmt.Errorf("%s: %w", n, err), 1)
	}
	cli.HandleExitCoder(err)
}

// TODO: make this a BeforeFunc and change ctx.Command.Action for the non-re-exec part

// actionReExecWrapper returns a cli.ActionFunc that first checks if the re-exec flag
// is set, and if not, re-execs the command, with the flag set, and a stripped
// set of permissions.
func actionReExecWrapper(f cli.ActionFunc, opts ...reExecOpt) cli.ActionFunc {
	opts = append(defaultReExecOpts(), opts...)

	return func(ctx *cli.Context) (err error) {
		if ctx.Bool(reExecFlagName) {
			if sc, ok := spanContextFromEnv(); ok {
				// rather than starting a new span, fake it by adding span and trace ID to all logs
				ctx.Context, _ = log.S(ctx.Context, logrus.Fields{
					logfields.TraceID: sc.TraceID.String(),
					logfields.SpanID:  sc.SpanID.String(),
				})
			}
			return f(ctx)
		}

		conf := reExecConfig{
			ac:   _useAppContainer,
			lpac: _useLPAC,
		}
		for _, o := range opts {
			if err := o(&conf); err != nil {
				return fmt.Errorf("could not properly initialize re-exec config: %w", err)
			}
		}

		span := startSpan(ctx, ctx.App.Name+"::"+ctx.Command.FullName())
		defer span.End()
		defer func() { oc.SetSpanStatus(span, err) }()
		conf.updateEnvWithTracing(ctx.Context)

		cmd, cleanup, err := conf.cmd(ctx)
		if err != nil {
			return fmt.Errorf("could not create re-exec command: %w", err)
		}
		defer cleanup()

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("could not start command: %w", err)
		}
		return cmd.Wait()
	}
}
