package errors

import (
	"fmt"
	"io"

	"github.com/pkg/errors"
)

// Hresult is a type corresponding to the HRESULT error type used on Windows.
type Hresult int32

const (
	HrUnexpected   = Hresult(-2147418113) // 0x8000FFFF
	HrNotImpl      = Hresult(-2147467263) // 0x80004001
	HrInvalidArg   = Hresult(-2147024809) // 0x80070057
	HrPointer      = Hresult(-2147467261) // 0x80004003
	HrFail         = Hresult(-2147467259) // 0x80004005
	HrAccessDenied = Hresult(-2147024891) // 0x80070005

	HrVmcomputeInvalidJSON = Hresult(-1070137075) // 0xC037010D
)

type containerExistsError struct {
	ID string
}

func (e *containerExistsError) Error() string {
	return fmt.Sprintf("a container with the ID \"%s\" already exists", e.ID)
}

// NewContainerExistsError returns a *containerExistsError referring to the
// given ID.
func NewContainerExistsError(id string) *containerExistsError {
	return &containerExistsError{ID: id}
}

type containerDoesNotExistError struct {
	ID string
}

func (e *containerDoesNotExistError) Error() string {
	return fmt.Sprintf("a container with the ID \"%s\" does not exist", e.ID)
}

// NewContainerDoesNotExistError returns a *containerDoesNotExistError
// referring to the given ID.
func NewContainerDoesNotExistError(id string) *containerDoesNotExistError {
	return &containerDoesNotExistError{ID: id}
}

type processDoesNotExistError struct {
	Pid int
}

func (e *processDoesNotExistError) Error() string {
	return fmt.Sprintf("a process with the pid %d does not exist", e.Pid)
}

// NewProcessDoesNotExistError returns a *processDoesNotExistError referring to
// the given pid.
func NewProcessDoesNotExistError(pid int) *processDoesNotExistError {
	return &processDoesNotExistError{Pid: pid}
}

// StackTracer is an interface originating (but not exported) from the
// github.com/pkg/errors package. It defines something which can return a stack
// trace.
type StackTracer interface {
	StackTrace() errors.StackTrace
}

type baseHresultError struct {
	hresult Hresult
}

func (e *baseHresultError) Error() string {
	return fmt.Sprintf("HRESULT: 0x%x", uint32(e.Hresult()))
}
func (e *baseHresultError) Hresult() Hresult {
	return e.hresult
}

type wrappingHresultError struct {
	cause   error
	hresult Hresult
}

func (e *wrappingHresultError) Error() string {
	return fmt.Sprintf("HRESULT 0x%x", uint32(e.Hresult())) + ": " + e.Cause().Error()
}
func (e *wrappingHresultError) Hresult() Hresult {
	return e.hresult
}
func (e *wrappingHresultError) Cause() error {
	return e.cause
}
func (e *wrappingHresultError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			fmt.Fprintf(s, "%+v\n", e.Cause())
			return
		}
		fallthrough
	case 's':
		io.WriteString(s, e.Error())
	case 'q':
		fmt.Fprintf(s, "%q", e.Error())
	}
}
func (e *wrappingHresultError) StackTrace() errors.StackTrace {
	type stackTracer interface {
		StackTrace() errors.StackTrace
	}
	serr, ok := e.Cause().(stackTracer)
	if !ok {
		return nil
	}
	return serr.StackTrace()
}

// NewHresultError produces a new error with the given HRESULT.
func NewHresultError(hresult Hresult) error {
	return &baseHresultError{hresult: hresult}
}

// WrapHresult produces a new error with the given HRESULT and wrapping the
// given error.
func WrapHresult(e error, hresult Hresult) error {
	return &wrappingHresultError{
		cause:   e,
		hresult: hresult,
	}
}

// GetHresult interates through the error's cause stack (similiarly to how the
// Cause function in github.com/pkg/errors operates). At the first error it
// encounters which implements the Hresult() method, it return's that error's
// HRESULT. This allows errors higher up in the cause stack to shadow the
// HRESULTs of errors lower down.
func GetHresult(e error) (Hresult, error) {
	type hresulter interface {
		Hresult() Hresult
	}
	type causer interface {
		Cause() error
	}
	cause := e
	for cause != nil {
		herr, ok := cause.(hresulter)
		if ok {
			return herr.Hresult(), nil
		}
		cerr, ok := cause.(causer)
		if !ok {
			break
		}
		cause = cerr.Cause()
	}
	return -1, errors.Errorf("no HRESULT found in cause stack for error %s", e)
}
