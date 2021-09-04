// Package cmd provides functionality used to execute commands inside of containers
// or UVMs, and to connect an upstream client to those commands for handling in/out/err IO.
package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Microsoft/hcsshim/internal/cow"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/windows"
)

// Cmd represents a command being prepared or run in a process host.
type Cmd struct {
	// Host is the process host in which to launch the process.
	Host cow.ProcessHost

	// The OCI spec for the process.
	Spec *specs.Process

	// Standard IO streams to relay to/from the process.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Log provides a logrus entry to use in logging IO copying status.
	Log *logrus.Entry

	// Context provides a context that terminates the process when it is done.
	Context context.Context

	// CopyAfterExitTimeout is the amount of time after process exit we allow the
	// stdout, stderr relays to continue before forcibly closing them if not
	// already completed. This is primarily a safety step against the HCS when
	// it fails to send a close on the stdout, stderr pipes when the process
	// exits and blocks the relay wait groups forever.
	CopyAfterExitTimeout time.Duration

	// Process is filled out after Start() returns.
	Process cow.Process

	// ExitState is filled out after Wait() (or Run() or Output()) completes.
	ExitState *ExitState

	iogrp     errgroup.Group
	stdinErr  atomic.Value
	allDoneCh chan struct{}
}

// ExitState contains whether a process has exited and with which exit code.
type ExitState struct {
	exited bool
	code   int
}

// ExitCode returns the exit code of the process, or -1 if the exit code is not known.
func (s *ExitState) ExitCode() int {
	if !s.exited {
		return -1
	}
	return s.code
}

// ExitError is used when a process exits with a non-zero exit code.
type ExitError struct {
	*ExitState
}

func (err *ExitError) Error() string {
	return fmt.Sprintf("process exited with exit code %d", err.ExitCode())
}

// Additional fields to hcsschema.ProcessParameters used by LCOW
type lcowProcessParameters struct {
	hcsschema.ProcessParameters
	OCIProcess *specs.Process `json:"OciProcess,omitempty"`
}

// escapeArgs makes a Windows-style escaped command line from a set of arguments
func escapeArgs(args []string) string {
	escapedArgs := make([]string, len(args))
	for i, a := range args {
		escapedArgs[i] = windows.EscapeArg(a)
	}
	return strings.Join(escapedArgs, " ")
}

// Command makes a Cmd for a given command and arguments.
func Command(host cow.ProcessHost, name string, arg ...string) *Cmd {
	cmd := &Cmd{
		Host: host,
		Spec: &specs.Process{
			Args: append([]string{name}, arg...),
		},
		Log:       logrus.NewEntry(logrus.StandardLogger()),
		ExitState: &ExitState{},
	}
	if host.OS() == "windows" {
		cmd.Spec.Cwd = `C:\`
	} else {
		cmd.Spec.Cwd = "/"
		cmd.Spec.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	}
	return cmd
}

// CommandContext makes a Cmd for a given command and arguments. After
// it is launched, the process is killed when the context becomes done.
func CommandContext(ctx context.Context, host cow.ProcessHost, name string, arg ...string) *Cmd {
	cmd := Command(host, name, arg...)
	cmd.Context = ctx
	return cmd
}

// relayIO is a glorified io.Copy that also logs when the copy has completed.
func relayIO(w io.Writer, r io.Reader, log *logrus.Entry, name string) (int64, error) {
	n, err := io.Copy(w, r)
	if log != nil {
		lvl := logrus.DebugLevel
		log = log.WithFields(logrus.Fields{
			"file":  name,
			"bytes": n,
		})
		if err != nil {
			lvl = logrus.ErrorLevel
			log = log.WithError(err)
		}
		log.Log(lvl, "Cmd IO relay complete")
	}
	return n, err
}

// Start starts a command. The caller must ensure that if Start succeeds,
// Wait is eventually called to clean up resources.
func (c *Cmd) Start() error {
	c.allDoneCh = make(chan struct{})
	var x interface{}
	if !c.Host.IsOCI() {
		wpp := &hcsschema.ProcessParameters{
			CommandLine:      c.Spec.CommandLine,
			User:             c.Spec.User.Username,
			WorkingDirectory: c.Spec.Cwd,
			EmulateConsole:   c.Spec.Terminal,
			CreateStdInPipe:  c.Stdin != nil,
			CreateStdOutPipe: c.Stdout != nil,
			CreateStdErrPipe: c.Stderr != nil,
		}

		if c.Spec.CommandLine == "" {
			if c.Host.OS() == "windows" {
				wpp.CommandLine = escapeArgs(c.Spec.Args)
			} else {
				wpp.CommandArgs = c.Spec.Args
			}
		}

		environment := make(map[string]string)
		for _, v := range c.Spec.Env {
			s := strings.SplitN(v, "=", 2)
			if len(s) == 2 && len(s[1]) > 0 {
				environment[s[0]] = s[1]
			}
		}
		wpp.Environment = environment

		if c.Spec.ConsoleSize != nil {
			wpp.ConsoleSize = []int32{
				int32(c.Spec.ConsoleSize.Height),
				int32(c.Spec.ConsoleSize.Width),
			}
		}
		x = wpp
	} else {
		lpp := &lcowProcessParameters{
			ProcessParameters: hcsschema.ProcessParameters{
				CreateStdInPipe:  c.Stdin != nil,
				CreateStdOutPipe: c.Stdout != nil,
				CreateStdErrPipe: c.Stderr != nil,
			},
			OCIProcess: c.Spec,
		}
		x = lpp
	}
	if c.Context != nil && c.Context.Err() != nil {
		return c.Context.Err()
	}
	p, err := c.Host.CreateProcess(context.TODO(), x)
	if err != nil {
		return err
	}
	c.Process = p
	if c.Log != nil {
		c.Log = c.Log.WithField("pid", p.Pid())
	}

	// Start relaying process IO.
	stdin, stdout, stderr := p.Stdio()
	if c.Stdin != nil {
		// Do not make stdin part of the error group because there is no way for
		// us or the caller to reliably unblock the c.Stdin read when the
		// process exits.
		go func() {
			_, err := relayIO(stdin, c.Stdin, c.Log, "stdin")
			// Report the stdin copy error. If the process has exited, then the
			// caller may never see it, but if the error was due to a failure in
			// stdin read, then it is likely the process is still running.
			if err != nil {
				c.stdinErr.Store(err)
			}
			// Notify the process that there is no more input.
			if err := p.CloseStdin(context.TODO()); err != nil && c.Log != nil {
				c.Log.WithError(err).Warn("failed to close Cmd stdin")
			}
		}()
	}

	if c.Stdout != nil {
		c.iogrp.Go(func() error {
			_, err := relayIO(c.Stdout, stdout, c.Log, "stdout")
			if err := p.CloseStdout(context.TODO()); err != nil {
				c.Log.WithError(err).Warn("failed to close Cmd stdout")
			}
			return err
		})
	}

	if c.Stderr != nil {
		c.iogrp.Go(func() error {
			_, err := relayIO(c.Stderr, stderr, c.Log, "stderr")
			if err := p.CloseStderr(context.TODO()); err != nil {
				c.Log.WithError(err).Warn("failed to close Cmd stderr")
			}
			return err
		})
	}

	if c.Context != nil {
		go func() {
			select {
			case <-c.Context.Done():
				_, _ = c.Process.Kill(context.TODO())
			case <-c.allDoneCh:
			}
		}()
	}
	return nil
}

// Wait waits for a command and its IO to complete and closes the underlying
// process. It can only be called once. It returns an ExitError if the command
// runs and returns a non-zero exit code.
func (c *Cmd) Wait() error {
	waitErr := c.Process.Wait()
	if waitErr != nil && c.Log != nil {
		c.Log.WithError(waitErr).Warn("process wait failed")
	}
	state := &ExitState{}
	code, exitErr := c.Process.ExitCode()
	if exitErr == nil {
		state.exited = true
		state.code = code
	}
	// Terminate the IO if the copy does not complete in the requested time.
	if c.CopyAfterExitTimeout != 0 {
		go func() {
			t := time.NewTimer(c.CopyAfterExitTimeout)
			defer t.Stop()
			select {
			case <-c.allDoneCh:
			case <-t.C:
				// Close the process to cancel any reads to stdout or stderr.
				c.Process.Close()
				if c.Log != nil {
					c.Log.Warn("timed out waiting for stdio relay")
				}
			}
		}()
	}
	ioErr := c.iogrp.Wait()
	if ioErr == nil {
		ioErr, _ = c.stdinErr.Load().(error)
	}
	close(c.allDoneCh)
	c.Process.Close()
	c.ExitState = state
	if exitErr != nil {
		return exitErr
	}
	if state.exited && state.code != 0 {
		return &ExitError{state}
	}
	return ioErr
}

// Run is equivalent to Start followed by Wait.
func (c *Cmd) Run() error {
	err := c.Start()
	if err != nil {
		return err
	}
	return c.Wait()
}

// Output runs a command via Run and collects its stdout into a buffer,
// which it returns.
func (c *Cmd) Output() ([]byte, error) {
	var b bytes.Buffer
	c.Stdout = &b
	err := c.Run()
	return b.Bytes(), err
}
