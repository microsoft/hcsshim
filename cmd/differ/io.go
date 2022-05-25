//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const pipePrefix = `\\.\pipe\` + appName + "-"

type ioPipes struct {
	names []string
	our   [3]*os.File
	proc  [3]*os.File
}

func newIOPipes(sd *windows.SECURITY_DESCRIPTOR) (_ *ioPipes, err error) {
	ps := &ioPipes{}
	defer func() {
		if err != nil {
			ps.Close()
		}
	}()

	for i, read := range [3]bool{true, false, false} {
		r, w, err := pipe(sd)
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

func (ps *ioPipes) Close() error {
	for _, ps := range [2][3]*os.File{ps.our, ps.proc} {
		for i, c := range ps {
			// fmt.Println("closing pipe", i)
			if c == nil {
				continue
			}
			if err := c.Close(); err != nil {
				return err
			}
			ps[i] = nil
		}
	}
	return nil
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
		// i := i
		r, w := rw.r, rw.w
		go func() {
			_, err := io.Copy(w, r)
			// fmt.Println("finished copy", i)
			errCh <- err
		}()
	}

	for range rws {
		if e := <-errCh; e != nil && !errors.Is(e, os.ErrClosed) {
			err = e
			fmt.Printf("got stdio error %e", err)
		}
	}
	fmt.Println("stdio for all finshed")
	return err
}
func pipe(sd *windows.SECURITY_DESCRIPTOR) (*os.File, *os.File, error) {
	var r, w windows.Handle
	sa := &windows.SecurityAttributes{
		SecurityDescriptor: sd,
		InheritHandle:      1,
	}
	sa.Length = uint32(unsafe.Sizeof(sa))
	if err := windows.CreatePipe(&r, &w, sa, 0); err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(r), "|0"), os.NewFile(uintptr(w), "|1"), nil
}

// func namedPipes() (string, *os.File, *os.File, error) {
// 	// get random name
// 	g, err := guid.NewV4()
// 	if err != nil {
// 		return "", nil, nil, fmt.Errorf("GUID V4 create: %w", err)
// 	}
// 	n := pipePrefix + g.String()
// 	pconf := &winio.PipeConfig{}
// 	l, err := winio.ListenPipe(n, pconf)
// 	if err != nil {
// 		return "", nil, nil, fmt.Errorf("dial pipe %q: %w", n, err)
// 	}
// 	r, err := l.Accept()

// 	return "", r, nil, nil
// }
