//go:build windows

package main

import (
	gcontext "context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/appargs"
	cmdpkg "github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/runhcs"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func newFile(context *cli.Context, param string) *os.File {
	fd := uintptr(context.Int(param))
	if fd == 0 {
		return nil
	}
	return os.NewFile(fd, "")
}

var shimCommand = cli.Command{
	Name:   "shim",
	Usage:  `launch the process and proxy stdio (do not call it outside of runhcs)`,
	Hidden: true,
	Flags: []cli.Flag{
		&cli.IntFlag{Name: "stdin", Hidden: true},
		&cli.IntFlag{Name: "stdout", Hidden: true},
		&cli.IntFlag{Name: "stderr", Hidden: true},
		&cli.BoolFlag{Name: "exec", Hidden: true},
		cli.StringFlag{Name: "log-pipe", Hidden: true},
	},
	Before: appargs.Validate(argID),
	Action: func(context *cli.Context) error {
		logPipe := context.String("log-pipe")
		if logPipe != "" {
			lpc, err := winio.DialPipe(logPipe, nil)
			if err != nil {
				return err
			}
			defer lpc.Close()
			logrus.SetOutput(lpc)
		} else {
			logrus.SetOutput(os.Stderr)
		}
		fatalWriter.Writer = os.Stdout

		id := context.Args().First()
		c, err := getContainer(id, true)
		if err != nil {
			return err
		}
		defer c.Close()

		// Asynchronously wait for the container to exit.
		containerExitCh := make(chan struct{})
		var containerExitErr error
		go func() {
			containerExitErr = c.hc.Wait()
			close(containerExitCh)
		}()

		// Get File objects for the open stdio files passed in as arguments.
		stdin := newFile(context, "stdin")
		stdout := newFile(context, "stdout")
		stderr := newFile(context, "stderr")

		exec := context.Bool("exec")
		terminateOnFailure := false

		errorOut := io.WriteCloser(os.Stdout)

		var spec *specs.Process

		if exec {
			// Read the process spec from stdin.
			specj, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			os.Stdin.Close()

			spec = new(specs.Process)
			err = json.Unmarshal(specj, spec)
			if err != nil {
				return err
			}

		} else {
			// Stdin is not used.
			os.Stdin.Close()

			// Listen on the named pipe associated with this container.
			l, err := winio.ListenPipe(c.ShimPipePath(), nil)
			if err != nil {
				return err
			}

			// Alert the parent process that initialization has completed
			// successfully.
			_, _ = errorOut.Write(runhcs.ShimSuccess)
			errorOut.Close()
			fatalWriter.Writer = io.Discard

			// When this process exits, clear this process's pid in the registry.
			defer func() {
				_ = stateKey.Set(id, keyShimPid, 0)
			}()

			defer func() {
				if terminateOnFailure {
					_ = c.hc.Terminate(gcontext.Background())
					<-containerExitCh
				}
			}()
			terminateOnFailure = true

			// Wait for a connection to the named pipe, exiting if the container
			// exits before this happens.
			var pipe net.Conn
			pipeCh := make(chan error)
			go func() {
				var err error
				pipe, err = l.Accept()
				pipeCh <- err
			}()

			select {
			case err = <-pipeCh:
				if err != nil {
					return err
				}
			case <-containerExitCh:
				err = containerExitErr
				if err != nil {
					return err
				}
				return cli.NewExitError("", 1)
			}

			// The next set of errors goes to the open pipe connection.
			errorOut = pipe
			fatalWriter.Writer = pipe

			// The process spec comes from the original container spec.
			spec = c.Spec.Process
		}

		// Create the process in the container.
		cmd := &cmdpkg.Cmd{
			Host:   c.hc,
			Stdin:  stdin,
			Stdout: stdout,
			Stderr: stderr,
		}
		if c.Spec.Linux == nil || exec {
			cmd.Spec = spec
		}

		err = cmd.Start()
		if err != nil {
			return err
		}
		pid := cmd.Process.Pid()
		if !exec {
			err = stateKey.Set(c.ID, keyInitPid, pid)
			if err != nil {
				stdin.Close()
				_, _ = cmd.Process.Kill(gcontext.Background())
				_ = cmd.Wait()
				return err
			}
		}

		// Store the Guest pid map
		err = stateKey.Set(c.ID, fmt.Sprintf(keyPidMapFmt, os.Getpid()), pid)

		if err != nil {
			stdin.Close()
			_, _ = cmd.Process.Kill(gcontext.Background())
			_ = cmd.Wait()
			return err
		}
		defer func() {
			// Remove the Guest pid map when this process is cleaned up
			_ = stateKey.Clear(c.ID, fmt.Sprintf(keyPidMapFmt, os.Getpid()))
		}()

		terminateOnFailure = false

		// Alert the connected process that the process was launched
		// successfully.
		_, _ = errorOut.Write(runhcs.ShimSuccess)
		errorOut.Close()
		fatalWriter.Writer = io.Discard

		_ = cmd.Wait()
		code := cmd.ExitState.ExitCode()
		if !exec {
			// Shutdown the container, waiting 5 minutes before terminating is
			// forcefully.
			const shutdownTimeout = time.Minute * 5
			err := c.hc.Shutdown(gcontext.Background())
			if err != nil {
				select {
				case <-containerExitCh:
					err = containerExitErr
				case <-time.After(shutdownTimeout):
					err = hcs.ErrTimeout
				}
			}

			if err != nil {
				_ = c.hc.Terminate(gcontext.Background())
			}
			<-containerExitCh
		}

		return cli.NewExitError("", code)
	},
}
