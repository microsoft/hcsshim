package gcserr

import (
	"errors"
	"fmt"
	"io"
)

// Hresult is a type corresponding to the HRESULT error type used on Windows.
type Hresult int32

// HRESULT constants matching internal\hcs\errors.go
// from MS-ERREF and HCS docs
const (
	// HrNotImpl is the HRESULT for a not implemented function.
	HrNotImpl = Hresult(-2147467263) // 0x80004001
	// HrFail is the HRESULT for an invocation or unspecified failure.
	HrFail = Hresult(-2147467259) // 0x80004005
	// HrErrNotFound is the HRESULT for an invalid process id.
	HrErrNotFound = Hresult(-2147023728) // 0x80070490
	// HrErrInvalidArg is the HRESULT for One or more arguments are invalid.
	HrErrInvalidArg = Hresult(-2147024809) // 0x80070057
	// HvVmcomputeTimeout is the HRESULT for operations that timed out.
	HvVmcomputeTimeout = Hresult(-1070137079) // 0xC0370109
	// HrVmcomputeInvalidJSON is the HRESULT for failing to unmarshal a json
	// string.
	HrVmcomputeInvalidJSON = Hresult(-1070137075) // 0xC037010D
	// HrVmcomputeSystemNotFound is the HRESULT for:
	//
	// A virtual machine or container with the specified identifier does not
	// exist.
	HrVmcomputeSystemNotFound = Hresult(-1070137074) // 0xC037010E
	// HrVmcomputeSystemAlreadyExists is the HRESULT for:
	//
	// A virtual machine or container with the specified identifier already exists.
	HrVmcomputeSystemAlreadyExists = Hresult(-1070137073) // 0xC037010F
	// HrVmcomputeUnsupportedProtocolVersion is the HRESULT for an invalid
	// protocol version range specified at negotiation.
	HrVmcomputeUnsupportedProtocolVersion = Hresult(-1070137076) // 0xC037010C
	// HrVmcomputeUnknownMessage is the HRESULT for unknown message types sent
	// from the HCS.
	HrVmcomputeUnknownMessage = Hresult(-1070137077) // 0xC037010B
	// HrVmcomputeInvalidState is the HRESULT for:
	//
	// The requested virtual machine or container operation is not valid in the
	// current state.
	HrVmcomputeInvalidState = Hresult(-2143878907) // 0x80370105
	// HrVmcomputeSystemAlreadyStopped is the HRESULT for:
	//
	// The virtual machine or container with the specified identifier is not
	// running.
	HrVmcomputeSystemAlreadyStopped = Hresult(-1070137072) // 0xC0370110
)

// Hresulter interface for errors with Hresult().
type Hresulter interface {
	Hresult() Hresult
}

// baseHresultError is a basic HRESULT error.
type baseHresultError struct {
	hresult Hresult
}

func (e *baseHresultError) Error() string {
	return fmt.Sprintf("HRESULT: 0x%x", uint32(e.Hresult()))
}

func (e *baseHresultError) Hresult() Hresult {
	return e.hresult
}

func (e *baseHresultError) Unwrap() error {
	return nil
}

// wrappingHresultError wraps another error with an HRESULT.
type wrappingHresultError struct {
	cause   error
	hresult Hresult
}

func (e *wrappingHresultError) Error() string {
	return fmt.Sprintf("HRESULT 0x%x: %s", uint32(e.Hresult()), e.cause.Error())
}

func (e *wrappingHresultError) Hresult() Hresult {
	return e.hresult
}

func (e *wrappingHresultError) Cause() error {
	return e.cause
}

func (e *wrappingHresultError) Unwrap() error {
	return e.cause
}

func (e *wrappingHresultError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			fmt.Fprintf(s, "%+v", e.Unwrap())
			return
		}
		fallthrough
	case 's', 'q':
		_, _ = io.WriteString(s, e.Error())
	}
}

// NewHresultError produces a new error with the given HRESULT.
func NewHresultError(hresult Hresult) error {
	return &baseHresultError{hresult: hresult}
}

// WrapHresult produces a new error with the given HRESULT wrapping the given error.
func WrapHresult(cause error, hresult Hresult) error {
	return &wrappingHresultError{cause: cause, hresult: hresult}
}

// GetHresult finds the first Hresult in the error chain using errors.As compatible loop.
func GetHresult(e error) (Hresult, error) {
	for e != nil {
		if herr, ok := e.(Hresulter); ok {
			return herr.Hresult(), nil
		}
		e = errors.Unwrap(e)
	}
	return 0, fmt.Errorf("no HRESULT found in error chain: %w", e)
}

// BaseStackTrace is removed as pkg/errors.StackTrace is deprecated.
// Stack traces can be obtained using %+v formatting on wrapped errors.
// TODO: if runtime/callers stack needed, add custom stack capturer.
