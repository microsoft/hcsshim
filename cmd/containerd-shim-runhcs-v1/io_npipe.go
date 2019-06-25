package main

import (
	"context"
	"io"
	"sync"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/oc"
	"go.opencensus.io/trace"
)

// newNpipeIO creates connected upstream io. It is the callers responsibility to
// validate that `if terminal == true`, `stderr == ""`.
func newNpipeIO(ctx context.Context, stdin, stdout, stderr string, terminal bool) (_ upstreamIO, err error) {
	ctx, span := trace.StartSpan(ctx, "newNpipeIO")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("stdin", stdin),
		trace.StringAttribute("stdout", stdout),
		trace.StringAttribute("stderr", stderr),
		trace.BoolAttribute("terminal", terminal))

	nio := &npipeio{
		stdin:    stdin,
		stdout:   stdout,
		stderr:   stderr,
		terminal: terminal,
	}
	defer func() {
		if err != nil {
			nio.Close(ctx)
		}
	}()
	if stdin != "" {
		c, err := winio.DialPipe(stdin, nil)
		if err != nil {
			return nil, err
		}
		nio.sin = c
	}
	if stdout != "" {
		c, err := winio.DialPipe(stdout, nil)
		if err != nil {
			return nil, err
		}
		nio.sout = c
	}
	if stderr != "" {
		c, err := winio.DialPipe(stderr, nil)
		if err != nil {
			return nil, err
		}
		nio.serr = c
	}
	return nio, nil
}

var _ = (upstreamIO)(&npipeio{})

type npipeio struct {
	// stdin, stdout, stderr are the original paths used to open the connections.
	//
	// They MUST be treated as readonly in the lifetime of the pipe io.
	stdin, stdout, stderr string
	// terminal is the original setting passed in on open.
	//
	// This MUST be treated as readonly in the lifetime of the pipe io.
	terminal bool

	// sin is the upstream `stdin` connection.
	//
	// `sin` MUST be treated as readonly in the lifetime of the pipe io after
	// the return from `newNpipeIO`.
	sin       io.ReadCloser
	sinCloser sync.Once

	// sout and serr are the upstream `stdout` and `stderr` connections.
	//
	// `sout` and `serr` MUST be treated as readonly in the lifetime of the pipe
	// io after the return from `newNpipeIO`.
	sout, serr   io.WriteCloser
	outErrCloser sync.Once
}

func (nio *npipeio) Close(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "npipeio::Close")
	defer span.End()

	nio.sinCloser.Do(func() {
		if nio.sin != nil {
			nio.sin.Close()
		}
	})
	nio.outErrCloser.Do(func() {
		if nio.sout != nil {
			nio.sout.Close()
		}
		if nio.serr != nil {
			nio.serr.Close()
		}
	})
}

func (nio *npipeio) CloseStdin(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "npipeio::CloseStdin")
	defer span.End()

	nio.sinCloser.Do(func() {
		if nio.sin != nil {
			nio.sin.Close()
		}
	})
}

func (nio *npipeio) Stdin() io.Reader {
	return nio.sin
}

func (nio *npipeio) StdinPath() string {
	return nio.stdin
}

func (nio *npipeio) Stdout() io.Writer {
	return nio.sout
}

func (nio *npipeio) StdoutPath() string {
	return nio.stdout
}

func (nio *npipeio) Stderr() io.Writer {
	return nio.serr
}

func (nio *npipeio) StderrPath() string {
	return nio.stderr
}

func (nio *npipeio) Terminal() bool {
	return nio.terminal
}
