package main

import (
	"context"
	"io"
	"sync"

	winio "github.com/Microsoft/go-winio"
	"github.com/sirupsen/logrus"
)

// newNpipeIO creates connected upstream io for task/exec `tid,eid`. It is the
// callers responsibility to validate that `if terminal == true`, `stderr ==
// ""`.
func newNpipeIO(ctx context.Context, tid, eid string, stdin, stdout, stderr string, terminal bool) (_ upstreamIO, err error) {
	logrus.WithFields(logrus.Fields{
		"tid":      tid,
		"eid":      eid,
		"stdin":    stdin,
		"stdout":   stdout,
		"stderr":   stderr,
		"terminal": terminal,
	}).Debug("npipeio::New")

	nio := &npipeio{
		stdin:    stdin,
		stdout:   stdout,
		stderr:   stderr,
		terminal: terminal,
	}
	defer func() {
		if err != nil {
			nio.Close()
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
	// tid, eid are the task and exec id's associated with this pipe io.
	tid, eid string
	// stdin, stdout, stderr are the original paths used to open the connections.
	//
	// They MUST be treated as readonly in the lifetime of the pipe io.
	stdin, stdout, stderr string
	// terminal is the original setting passed in on open.
	//
	// This MUST be treated as readonly in the lifetime of the pipe io.
	terminal bool

	// l synchronizes access to all io connections.
	l sync.Mutex

	// sin is the upstream `stdin` connection.
	sin io.ReadCloser
	// sout and serr are the upstream `stdout` and `stderr` connections.
	sout, serr io.WriteCloser
}

func (nio *npipeio) Close() {
	logrus.WithFields(logrus.Fields{
		"tid": nio.tid,
		"eid": nio.eid,
	}).Debug("npipeio::Close")

	nio.l.Lock()
	defer nio.l.Unlock()

	if nio.sin != nil {
		nio.sin.Close()
		nio.sin = nil
	}
	if nio.sout != nil {
		nio.sout.Close()
		nio.sout = nil
	}
	if nio.serr != nil {
		nio.serr.Close()
		nio.serr = nil
	}
}

func (nio *npipeio) CloseStdin() {
	logrus.WithFields(logrus.Fields{
		"tid": nio.tid,
		"eid": nio.eid,
	}).Debug("npipeio::CloseStdin")

	nio.l.Lock()
	defer nio.l.Unlock()

	if nio.sin != nil {
		nio.sin.Close()
		nio.sin = nil
	}
}

func (nio *npipeio) Stdin() io.Reader {
	nio.l.Lock()
	defer nio.l.Unlock()

	return nio.sin
}

func (nio *npipeio) StdinPath() string {
	return nio.stdin
}

func (nio *npipeio) Stdout() io.Writer {
	nio.l.Lock()
	defer nio.l.Unlock()

	return nio.sout
}

func (nio *npipeio) StdoutPath() string {
	return nio.stdout
}

func (nio *npipeio) Stderr() io.Writer {
	nio.l.Lock()
	defer nio.l.Unlock()

	return nio.serr
}

func (nio *npipeio) StderrPath() string {
	return nio.stderr
}

func (nio *npipeio) Terminal() bool {
	return nio.terminal
}
