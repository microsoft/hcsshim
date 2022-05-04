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
do not inherit handles, only open new handles for stdin/out/err, and files/directories needed

todo:
before re-exec would need runtime parameters from payload (ie, tracing, files to allow access to)
intercept pipe and pass along command-specific payload to re-exec
do not allow access to upstream containerd pipe for payload
*/

var appCommands = []*cli.Command{
	decompressCommand,
	convertCommand,
	wclayerCommand,
	testCommand,
}

func app() *cli.App {
	app := &cli.App{
		Name:           "hcsshim-differ",
		Usage:          "Containerd stream processors for applying for Windows container (WCOW and LCOW) diffs and layers",
		Commands:       appCommands,
		ExitErrHandler: errHandler,
		Before:         beforeApp,
		Flags: []cli.Flag{
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

// actionReExecWrapper returns a cli.ActionFunc that first checks if the re-exec flag
// is set, and if not, re-execs the command, with the flag set, and a stripped
// set of permissions. If r != nil, it will be run after creating the cmd to re-exec
func actionReExecWrapper(f cli.ActionFunc, opts ...reExecOpt) cli.ActionFunc {
	conf := reExecConfig{}
	var confErr error // cant return an error here, so punt error checking till action
	opts = append(defaultReExecOpts(), opts...)
	for _, o := range opts {
		if confErr := o(&conf); confErr != nil {
			break
		}
	}
	return func(c *cli.Context) (err error) {
		if confErr != nil {
			return fmt.Errorf("could not properly initialize re-exec config: %w", confErr)
		}

		if c.Bool(reExecFlagName) {
			if sc, ok := spanContextFromEnv(); ok {
				// rather than starting a new span, fake it by adding span and trace ID to all logs
				c.Context, _ = log.S(c.Context, logrus.Fields{
					logfields.TraceID: sc.TraceID.String(),
					logfields.SpanID:  sc.SpanID.String(),
				})
			}
			return f(c)
		}

		span := startSpan(c, c.App.Name+"::"+c.Command.FullName())
		defer span.End()
		defer func() { oc.SetSpanStatus(span, err) }()

		cmd, cleanup, err := conf.cmd(c.Context)
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
