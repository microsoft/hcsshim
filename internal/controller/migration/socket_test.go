//go:build windows && lcow

package migration

import (
	"context"
	"errors"
	"testing"
	"unsafe"

	"github.com/containerd/errdefs"
	"golang.org/x/sys/windows"
)

// validProtocolInfo returns a correctly-sized (all-zero) serialized descriptor
// that passes the size check and decode, so tests can exercise the later guards.
func validProtocolInfo() []byte {
	return make([]byte, int(unsafe.Sizeof(windows.WSAProtocolInfo{})))
}

// TestRegisterDuplicateSocket_TooSmall verifies a buffer too short to hold a
// socket descriptor is rejected as an invalid argument before any decode.
func TestRegisterDuplicateSocket_TooSmall(t *testing.T) {
	c := New()

	err := c.RegisterDuplicateSocket(context.Background(), "", []byte{0x00})
	if !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("got %v, want ErrInvalidArgument", err)
	}
}

// TestRegisterDuplicateSocket_SessionMismatch verifies a call for a session
// other than the active one is rejected as an invalid argument.
func TestRegisterDuplicateSocket_SessionMismatch(t *testing.T) {
	c := New()
	c.sessionID = "active"

	err := c.RegisterDuplicateSocket(context.Background(), "other", validProtocolInfo())
	if !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("got %v, want ErrInvalidArgument", err)
	}
}

// TestRegisterDuplicateSocket_Idempotent verifies a repeat call once a socket
// has been adopted is a no-op, regardless of state.
func TestRegisterDuplicateSocket_Idempotent(t *testing.T) {
	c := New()
	c.sessionID = "s"
	c.dupSocket = windows.Handle(1)
	c.state = StateSourceExported

	if err := c.RegisterDuplicateSocket(context.Background(), "s", validProtocolInfo()); err != nil {
		t.Fatalf("got %v, want nil", err)
	}
}

// TestRegisterDuplicateSocket_InvalidState verifies registration is rejected as
// a failed precondition when the session is not awaiting a socket.
func TestRegisterDuplicateSocket_InvalidState(t *testing.T) {
	c := New()
	c.sessionID = "s"
	c.state = StateIdle

	err := c.RegisterDuplicateSocket(context.Background(), "s", validProtocolInfo())
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("got %v, want ErrFailedPrecondition", err)
	}
}
