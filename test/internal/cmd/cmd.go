//go:build windows

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

func desc(c *cmd.Cmd) string {
	desc := "init command"
	if c.Spec != nil {
		desc = strings.Join(c.Spec.Args, " ")
	}

	return desc
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

func Start(_ context.Context, t testing.TB, c *cmd.Cmd) {
	if err := c.Start(); err != nil {
		t.Helper()
		t.Fatalf("failed to start %q: %v", desc(c), err)
	}
}

func Run(ctx context.Context, t testing.TB, c *cmd.Cmd) int {
	Start(ctx, t, c)
	e := Wait(ctx, t, c)

	return e
}

func Wait(_ context.Context, t testing.TB, c *cmd.Cmd) int {
	// todo, wait on context.Done
	if err := c.Wait(); err != nil {
		t.Helper()

		ee := &cmd.ExitError{}
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		t.Fatalf("failed to wait on %q: %v", desc(c), err)
	}

	return 0
}

func WaitExitCode(ctx context.Context, t testing.TB, c *cmd.Cmd, e int) {
	if ee := Wait(ctx, t, c); ee != e {
		t.Helper()
		t.Errorf("got exit code %d, wanted %d", ee, e)
	}
}

func Kill(ctx context.Context, t testing.TB, c *cmd.Cmd) {
	ok, err := c.Process.Kill(ctx)
	if !ok {
		t.Helper()
		t.Fatalf("could not deliver kill to %q", desc(c))
	} else if err != nil {
		t.Helper()
		t.Fatalf("could not kill %q: %v", desc(c), err)
	}
}
