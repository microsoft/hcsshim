//go:build windows

package main

import (
	"errors"
	"io"
	"os"

	"github.com/Microsoft/hcsshim/internal/winapi"
	"golang.org/x/sys/windows"
)

type ioPipes struct {
	our  [3]*os.File
	proc [3]*os.File
}

func newIOPipes(sd *windows.SECURITY_DESCRIPTOR) (_ *ioPipes, err error) {
	ps := &ioPipes{}
	defer func() {
		if err != nil {
			ps.Close()
		}
	}()

	for i, read := range [3]bool{true, false, false} {
		r, w, err := winapi.CreatePipe(winapi.NewSecurityAttributes(sd, true), 0)
		if err != nil {
			return nil, err
		}
		if read {
			ps.our[i], ps.proc[i] = w, r
		} else {
			ps.our[i], ps.proc[i] = r, w
		}
	}
	return ps, nil
}

func (ps *ioPipes) Close() (err error) {
	for _, c := range append(ps.our[:], ps.proc[:]...) {
		if c == nil {
			continue
		}
		if cErr := c.Close(); cErr != nil && !errors.Is(err, os.ErrClosed) && err == nil {
			err = cErr
		}
	}
	ps.our = [3]*os.File{}
	ps.proc = [3]*os.File{}
	return err
}

func (ps *ioPipes) Copy() (err error) {
	errCh := make(chan error)
	defer close(errCh)

	rws := []struct {
		w io.Writer
		r io.Reader
	}{
		{ps.our[0], os.Stdin},
		{os.Stdout, ps.our[1]},
		{os.Stderr, ps.our[2]},
	}
	for _, rw := range rws {
		r, w := rw.r, rw.w
		go func() {
			_, err := io.Copy(w, r)
			errCh <- err
		}()
	}

	for range rws {
		if e := <-errCh; e != nil && !errors.Is(e, os.ErrClosed) {
			err = e
		}
	}
	return err
}
