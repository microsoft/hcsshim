//go:build windows

package hcs

import (
	"errors"
	"fmt"
	"net"
	"syscall"
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
			if !errors.As(nerr, &e) || e.S != err.S {
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

func TestIsAlreadyStopped(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "vmcompute already stopped (0xc0370110)",
			err:  ErrVmcomputeAlreadyStopped,
			want: true,
		},
		{
			// Compute system reported as no longer running (e.g. UVM stopped during migration teardown).
			name: "system already stopped (0x80370110)",
			err:  ErrVmcomputeSystemAlreadyStopped,
			want: true,
		},
		{
			name: "process already stopped",
			err:  ErrProcessAlreadyStopped,
			want: true,
		},
		{
			name: "element not found",
			err:  ErrElementNotFound,
			want: true,
		},
		{
			name: "system not running wrapped in SystemError",
			err: &SystemError{
				ID: "uvm-test",
				HcsError: HcsError{
					Op:  "hcs::System::Modify",
					Err: ErrVmcomputeSystemAlreadyStopped,
				},
			},
			want: true,
		},
		{
			name: "unrelated error",
			err:  syscall.Errno(0x5),
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAlreadyStopped(tc.err); got != tc.want {
				t.Errorf("IsAlreadyStopped(%v) = %t, want %t", tc.err, got, tc.want)
			}
		})
	}
}
