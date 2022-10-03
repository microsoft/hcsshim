//go:build windows

package hcs

import (
	"errors"
	"fmt"
	"net"
	"testing"
)

type MyError struct {
	S string
}

func (e *MyError) Error() string {
	return fmt.Sprintf("error happened: %s", e.S)
}

func TestHcsErrorUnwrap(t *testing.T) {
	err := &MyError{"test test"}
	herr := HcsError{
		Op:  t.Name(),
		Err: err,
	}

	for _, nerr := range []net.Error{
		&herr,
		&SystemError{
			ID:       t.Name(),
			HcsError: herr,
		},
		&ProcessError{
			SystemID: t.Name(),
			HcsError: herr,
		},
	} {
		t.Run(fmt.Sprintf("%T", nerr), func(t *testing.T) {
			if !errors.Is(nerr, err) {
				t.Errorf("error '%v' did not unwrap to %v", nerr, err)
			}

			var e *MyError
			if !(errors.As(nerr, &e) && e.S == err.S) {
				t.Errorf("error '%v' did not unwrap '%v' properly", errors.Unwrap(nerr), e)
			}

			if nerr.Timeout() {
				t.Errorf("expected .Timeout() on '%v' to be false", nerr)
			}

			//nolint:staticcheck // Temporary() is deprecated
			if nerr.Temporary() {
				t.Errorf("expected .Temporary() on '%v' to be false", nerr)
			}
		})
	}
}

func TestHcsErrorUnwrapTimeout(t *testing.T) {
	err := fmt.Errorf("error: %w", ErrTimeout)
	herr := HcsError{
		Op:  "test",
		Err: err,
	}

	for _, nerr := range []net.Error{
		&herr,
		&SystemError{
			ID:       t.Name(),
			HcsError: herr,
		},
		&ProcessError{
			SystemID: t.Name(),
			HcsError: herr,
		},
	} {
		t.Run(fmt.Sprintf("%T", nerr), func(t *testing.T) {
			if !errors.Is(nerr, ErrTimeout) {
				t.Errorf("error '%v' did not unwrap to %v", nerr, ErrTimeout)
			}

			if !errors.Is(nerr, err) {
				t.Errorf("error '%v' did not unwrap to %v", nerr, err)
			}

			if !IsTimeout(nerr) {
				t.Errorf("expected error '%v' to be timeout", nerr)
			}

			if nerr.Timeout() {
				t.Errorf("expected .Timeout() on '%v' to be false", nerr)
			}

			//nolint:staticcheck // Temporary() is deprecated
			if nerr.Temporary() {
				t.Errorf("expected .Temporary() on '%v' to be false", nerr)
			}
		})
	}
}

var errNet = netError{}

type netError struct{}

func (e netError) Error() string   { return "temporary timeout" }
func (e netError) Timeout() bool   { return true }
func (e netError) Temporary() bool { return true }

func TestHcsErrorUnwrapNet(t *testing.T) {
	err := fmt.Errorf("error: %w", errNet)
	herr := HcsError{
		Op:  "test",
		Err: err,
	}

	for _, nerr := range []net.Error{
		&herr,
		&SystemError{
			ID:       t.Name(),
			HcsError: herr,
		},
		&ProcessError{
			SystemID: t.Name(),
			HcsError: herr,
		},
	} {
		t.Run(fmt.Sprintf("%T", nerr), func(t *testing.T) {
			if !errors.Is(nerr, errNet) {
				t.Errorf("error '%v' did not unwrap to %v", nerr, errNet)
			}

			if !errors.Is(nerr, err) {
				t.Errorf("error '%v' did not unwrap to %v", nerr, err)
			}

			if !IsTimeout(nerr) {
				t.Errorf("expected error '%v' to be timeout", nerr)
			}

			if !nerr.Timeout() {
				t.Errorf("expected .Timeout() on '%v' to be true", nerr)
			}

			//nolint:staticcheck // Temporary() is deprecated
			if !nerr.Temporary() {
				t.Errorf("expected .Temporary() on '%v' to be true", nerr)
			}
		})
	}
}
