package main

import (
	"context"
	"io"
	"sync"

	winio "github.com/Microsoft/go-winio"
	"github.com/sirupsen/logrus"
)

func newRelay(ctx context.Context, stdin, stdout, stderr string, terminal bool) (_ *iorelay, err error) {
	logrus.WithFields(logrus.Fields{
		"stdin":    stdin,
		"stdout":   stdout,
		"stderr":   stderr,
		"terminal": terminal,
	}).Debug("newRelay")

	ir := &iorelay{
		stdin:    stdin,
		stdout:   stdout,
		stderr:   stderr,
		terminal: terminal,
	}
	defer func() {
		if err != nil {
			ir.close()
		}
	}()
	if stdin != "" {
		c, err := winio.DialPipe(stdin, nil)
		if err != nil {
			return nil, err
		}
		ir.ui = c
	}
	if stdout != "" {
		c, err := winio.DialPipe(stdout, nil)
		if err != nil {
			return nil, err
		}
		ir.uo = c
	}
	if stderr != "" {
		c, err := winio.DialPipe(stderr, nil)
		if err != nil {
			return nil, err
		}
		ir.ue = c
	}
	return ir, nil
}

type iorelay struct {
	stdin, stdout, stderr string
	terminal              bool

	// wg tracks the state of the relay's stdin/stdout/stderr if they exist.
	wg sync.WaitGroup

	// l synchronizes access to all upstream/downstream IO
	l sync.Mutex

	// ui is the upstream stdin. IE: above the shim.
	ui io.ReadCloser
	// uo and ue are the upstream stdout and stderr. IE: above the shim.
	uo, ue io.WriteCloser

	// di is the down stdin. IE: from the hcs process.
	di io.WriteCloser
	// do and de are the downstream stdout and stderr. IE: from the hcs process.
	do, de io.ReadCloser
}

func (ir *iorelay) StdinPath() string {
	return ir.stdin
}

func (ir *iorelay) StdoutPath() string {
	return ir.stdout
}

func (ir *iorelay) StderrPath() string {
	return ir.stderr
}

func (ir *iorelay) Terminal() bool {
	return ir.terminal
}

func (ir *iorelay) BeginRelay(stdin io.WriteCloser, stdout io.ReadCloser, stderr io.ReadCloser) {
	ir.l.Lock()
	defer ir.l.Unlock()

	if ir.ui != nil {
		ir.di = stdin
		go func() {
			io.Copy(ir.di, ir.ui)
		}()
	}
	if ir.uo != nil {
		ir.do = stdout
		ir.wg.Add(1)
		go func() {
			io.Copy(ir.uo, ir.do)
			ir.wg.Done()
		}()
	}
	if ir.ue != nil {
		ir.de = stderr
		ir.wg.Add(1)
		go func() {
			io.Copy(ir.ue, ir.de)
			ir.wg.Done()
		}()
	}
}

func (ir *iorelay) CloseStdin() {
	ir.l.Lock()
	defer ir.l.Unlock()
	if ir.ui != nil {
		ir.ui.Close()
		ir.ui = nil
	}
	if ir.di != nil {
		ir.di.Close()
		ir.di = nil
	}
}

func (ir *iorelay) Wait() {
	ir.wg.Wait()

	ir.close()
}

func (ir *iorelay) close() {
	ir.l.Lock()
	defer ir.l.Unlock()

	for _, closer := range []io.Closer{
		ir.ui, ir.di,
		ir.do, ir.uo,
		ir.de, ir.ue,
	} {
		if closer != nil {
			closer.Close()
		}
	}
	ir.ui = nil
	ir.uo = nil
	ir.ue = nil
	ir.di = nil
	ir.do = nil
	ir.de = nil
}
