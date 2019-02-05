package main

import (
	"io"
	"sync"

	winio "github.com/Microsoft/go-winio"
)

func newRelay(stdin, stdout, stderr string) (_ *iorelay, err error) {
	ir := &iorelay{}
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
	wg sync.WaitGroup

	l sync.Mutex

	ui     io.ReadCloser
	uo, ue io.WriteCloser

	di     io.WriteCloser
	do, de io.ReadCloser
}

func (ir *iorelay) BeginRelay(stdin io.WriteCloser, stdout io.ReadCloser, stderr io.ReadCloser) {
	ir.l.Lock()
	defer ir.l.Unlock()

	if ir.ui != nil {
		ir.di = stdin
		ir.wg.Add(1)
		go func() {
			io.Copy(ir.di, ir.ui)
			ir.wg.Done()
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

	ir.l.Lock()
	defer ir.l.Unlock()
	if ir.ui != nil {
		ir.ui.Close()
		ir.ui = nil
		ir.di.Close()
		ir.di = nil
	}
	if ir.uo != nil {
		ir.do.Close()
		ir.do = nil
		ir.uo.Close()
		ir.uo = nil
	}
	if ir.ue != nil {
		ir.de.Close()
		ir.de = nil
		ir.ue.Close()
		ir.ue = nil
	}
}

func (ir *iorelay) close() {
	ir.l.Lock()
	defer ir.l.Unlock()

	for _, closer := range []io.Closer{
		ir.ui, ir.di,
		ir.do, ir.uo,
		ir.de, ir.ue,
	} {
		closer.Close()
	}
	ir.ui = nil
	ir.uo = nil
	ir.ue = nil
}
