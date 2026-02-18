//go:build windows

// This package provides testing wrappers around [github.com/Microsoft/hcsshim/internal/cmd]
package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/log"
)

const CopyAfterExitTimeout = time.Second

// ForcedKilledExitCode is the (Linux) exit code when processes are foreably killed.
const ForcedKilledExitCode = 137

func desc(c *cmd.Cmd) string {
	switch {
	case c == nil:
		return "<nil>"
	case c.Spec == nil:
		return "init command"
	case c.Spec.CommandLine != "":
		return c.Spec.CommandLine
	default:
	}

	return strings.Join(c.Spec.Args, " ")
}

func Create(ctx context.Context, _ testing.TB, c cow.ProcessHost, p *specs.Process, io *BufferedIO) *cmd.Cmd {
	cc := &cmd.Cmd{
		Host:                 c,
		Context:              ctx,
		Spec:                 p,
		Log:                  log.G(ctx),
		CopyAfterExitTimeout: CopyAfterExitTimeout,
		ExitState:            &cmd.ExitState{},
	}
	io.AddToCmd(cc)

	return cc
}

func Start(_ context.Context, tb testing.TB, c *cmd.Cmd) {
	tb.Helper()

	d := desc(c)
	tb.Logf("starting command: %q", d)

	if err := c.Start(); err != nil {
		tb.Fatalf("failed to start %q: %v", d, err)
	}
}

func Run(ctx context.Context, tb testing.TB, c *cmd.Cmd) int {
	tb.Helper()
	Start(ctx, tb, c)
	return Wait(ctx, tb, c)
}

func Wait(_ context.Context, tb testing.TB, c *cmd.Cmd) int {
	tb.Helper()

	d := desc(c)
	tb.Logf("waiting on process: %q", d)

	// todo, wait on context.Done
	if err := c.Wait(); err != nil {
		ee := &cmd.ExitError{}
		if errors.As(err, &ee) {
			ec := ee.ExitCode()
			tb.Logf("process exit code: %d", ec)
			return ec
		}
		tb.Fatalf("failed to wait on %q: %v", d, err)
	}

	return 0
}

func WaitExitCode(ctx context.Context, tb testing.TB, c *cmd.Cmd, e int) {
	tb.Helper()
	if ee := Wait(ctx, tb, c); ee != e {
		tb.Errorf("got exit code %d, wanted %d", ee, e)
	}
}

func Kill(ctx context.Context, tb testing.TB, c *cmd.Cmd) {
	tb.Helper()

	d := desc(c)
	tb.Logf("kill process: %q", d)

	ok, err := c.Process.Kill(ctx)
	if !ok {
		tb.Fatalf("could not deliver kill to %q", d)
	} else if err != nil {
		tb.Fatalf("could not kill %q: %v", d, err)
	}
}
