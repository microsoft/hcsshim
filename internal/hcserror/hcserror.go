//go:build windows

package hcserror

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/windows"
)

const ERROR_GEN_FAILURE = syscall.Errno(31)

type HcsError struct {
	title string
	rest  string
	Err   error
}

func (e *HcsError) Error() string {
	s := e.title
	if len(s) > 0 && s[len(s)-1] != ' ' {
		s += " "
	}
	s += fmt.Sprintf("failed in Win32: %s (0x%x)", e.Err, Win32FromError(e.Err))
	if e.rest != "" {
		if e.rest[0] != ' ' {
			s += " "
		}
		s += e.rest
	}
	return s
}

func New(err error, title, rest string) error {
	// Pass through DLL errors directly since they do not originate from HCS.
	if t := (&windows.DLLError{}); errors.As(err, &t) {
		return err
	}
	return &HcsError{title, rest, err}
}

func Win32FromError(err error) uint32 {
	if herr := (&HcsError{}); errors.As(err, &herr) {
		return Win32FromError(herr.Err)
	}
	if code := (windows.Errno(0)); errors.As(err, &code) {
		return uint32(code)
	}
	return uint32(ERROR_GEN_FAILURE)
}

// Is is a vectorized version of errors.Is. It returns true if err is one of errs.
func Is(err error, errs ...error) bool {
	// TODO: replace with a fold/reduce if golang adds one to its std lib
	for _, e := range errs {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
