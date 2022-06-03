//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	osExec "os/exec"

	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/exec"
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
	for _, c := range appCommands {
		if c.Before == nil {
			c.Before = createCommandBeforeFunc()
		}
	}
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
	return nil
}

func errHandler(c *cli.Context, err error) {
	if err == nil {
		return
	}
	// reexec will return an exit code, so check for that edge case and
	if ee := (&osExec.ExitError{}); errors.As(err, &ee) {
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

// createCommandBeforeFunc returns a cli.BeforeFunc that first checks if the re-exec flag
// is set, and if not, creates a re-exec with the flag set, and changes the command's ActionFunc
// to run and wait for that command to finish.
func createCommandBeforeFunc(opts ...reExecOpt) cli.BeforeFunc {
	opts = append(defaultReExecOpts(), opts...)
	return func(ctx *cli.Context) error {
		if ctx.Bool(reExecFlagName) {
			if sc, ok := spanContextFromEnv(); ok {
				// rather than starting a new span, fake it by adding span and trace ID to all logs
				ctx.Context, _ = log.S(ctx.Context, logrus.Fields{
					logfields.TraceID: sc.TraceID.String(),
					logfields.SpanID:  sc.SpanID.String(),
				})
			}
			return nil
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
		conf.updateEnvWithTracing(ctx.Context)

		sids, err := conf.capabilitySIDs()
		if err != nil {
			return err
		}
		log.G(ctx.Context).WithFields(logrus.Fields{
			"sids": sids,
		}).Debug("Created capability SIDs")

		sd, err := conf.pipeSecurityDescriptor(sids, windows.GENERIC_ALL)
		if err != nil {
			return fmt.Errorf("create pipe security descriptor: %w", err)
		}
		pipes, err := newIOPipes(sd)
		if err != nil {
			return fmt.Errorf("create StdIO pipes: %w", err)
		}

		files := []string{filename, dirname}
		if err := grantSIDsFileAccess(sids, files, windows.GENERIC_READ|windows.GENERIC_EXECUTE|windows.GENERIC_WRITE); err != nil {
			return err
		}

		cmd, cleanup, err := conf.cmd(ctx, sids,
			exec.UsingStdio(pipes.proc[0], pipes.proc[1], pipes.proc[2]),
			exec.WithEnv(conf.env))
		if err != nil {
			return fmt.Errorf("create re-exec command: %w", err)
		}

		ctx.Command.Action = func(ctx *cli.Context) (err error) {
			defer span.End()
			defer func() { oc.SetSpanStatus(span, err) }()
			defer revokeSIDsFileAccess(sids, files) //nolint:errcheck
			defer cleanup()

			ch := make(chan error)
			go func() {
				defer close(ch)
				ch <- pipes.Copy()
			}()
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("could not start command: %w", err)
			}
			if err := cmd.Wait(); err != nil {
				return err
			}
			pipes.Close()
			os.Stdin.Close()
			return <-ch
		}
		return nil
	}
}
