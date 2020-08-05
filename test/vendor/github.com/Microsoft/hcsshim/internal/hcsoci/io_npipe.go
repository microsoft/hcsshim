package hcsoci

import (
	"context"
	"io"
	"sync"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/sirupsen/logrus"
)

// NewNpipeIO creates connected upstream io. It is the callers responsibility to
// validate that `if terminal == true`, `stderr == ""`.
func NewNpipeIO(ctx context.Context, stdin, stdout, stderr string, terminal bool) (_ UpstreamIO, err error) {
	log.G(ctx).WithFields(logrus.Fields{
		"stdin":    stdin,
		"stdout":   stdout,
		"stderr":   stderr,
		"terminal": terminal}).Debug("NewNpipeIO")

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
		c, err := winio.DialPipeContext(ctx, stdin)
		if err != nil {
			return nil, err
		}
		nio.sin = c
	}
	if stdout != "" {
		c, err := winio.DialPipeContext(ctx, stdout)
		if err != nil {
			return nil, err
		}
		nio.sout = c
	}
	if stderr != "" {
		c, err := winio.DialPipeContext(ctx, stderr)
		if err != nil {
			return nil, err
		}
		nio.serr = c
	}
	return nio, nil
}

var _ = (UpstreamIO)(&npipeio{})

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
	// the return from `NewNpipeIO`.
	sin       io.ReadCloser
	sinCloser sync.Once

	// sout and serr are the upstream `stdout` and `stderr` connections.
	//
	// `sout` and `serr` MUST be treated as readonly in the lifetime of the pipe
	// io after the return from `NewNpipeIO`.
	sout, serr   io.WriteCloser
	outErrCloser sync.Once
}

func (nio *npipeio) Close(ctx context.Context) {
	nio.sinCloser.Do(func() {
		if nio.sin != nil {
			log.G(ctx).Debug("npipeio::sinCloser")
			nio.sin.Close()
		}
	})
	nio.outErrCloser.Do(func() {
		if nio.sout != nil {
			log.G(ctx).Debug("npipeio::outErrCloser - stdout")
			nio.sout.Close()
		}
		if nio.serr != nil {
			log.G(ctx).Debug("npipeio::outErrCloser - stderr")
			nio.serr.Close()
		}
	})
}

func (nio *npipeio) CloseStdin(ctx context.Context) {
	nio.sinCloser.Do(func() {
		if nio.sin != nil {
			log.G(ctx).Debug("npipeio::sinCloser")
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
