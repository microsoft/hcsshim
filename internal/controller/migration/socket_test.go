//go:build windows && lcow

package migration

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
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

// TestRegisterDuplicateSocket_RejectsAlreadyRegistered verifies a repeat call
// once a socket has been adopted is rejected as already exists.
func TestRegisterDuplicateSocket_RejectsAlreadyRegistered(t *testing.T) {
	c := New()
	c.sessionID = "s"
	c.dupSocket = windows.Handle(1)
	c.state = StateSourceExported

	if err := c.RegisterDuplicateSocket(context.Background(), "s", validProtocolInfo()); !errors.Is(err, errdefs.ErrAlreadyExists) {
		t.Fatalf("got %v, want ErrAlreadyExists", err)
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

// TestProtocolInfoNativeLayout guards the assumption behind decoding the
// descriptor as raw native memory: it has no padding, so the native size we
// validate on the wire equals its packed size. A layout change adds padding.
func TestProtocolInfoNativeLayout(t *testing.T) {
	native := int(unsafe.Sizeof(windows.WSAProtocolInfo{}))
	packed := binary.Size(windows.WSAProtocolInfo{})
	if native != packed {
		t.Fatalf("native size %d != packed size %d; revisit the decode in RegisterDuplicateSocket", native, packed)
	}
}

// connectedSocketDescriptor returns a real connected socket's descriptor,
// serialized the way the caller transmits it: the descriptor's raw native
// memory. The source socket is kept alive for the duration of the test.
func connectedSocketDescriptor(t *testing.T) []byte {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	server, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	var handle windows.Handle
	raw, err := conn.(*net.TCPConn).SyscallConn()
	if err != nil {
		t.Fatalf("syscall conn: %v", err)
	}
	if err := raw.Control(func(fd uintptr) { handle = windows.Handle(fd) }); err != nil {
		t.Fatalf("control: %v", err)
	}

	// WSADuplicateSocket is exactly how the caller produces the descriptor.
	var info windows.WSAProtocolInfo
	if err := windows.WSADuplicateSocket(handle, windows.GetCurrentProcessId(), &info); err != nil {
		t.Fatalf("WSADuplicateSocket: %v", err)
	}

	b := make([]byte, unsafe.Sizeof(info))
	copy(b, unsafe.Slice((*byte)(unsafe.Pointer(&info)), unsafe.Sizeof(info)))
	return b
}

// TestRegisterDuplicateSocket_EndToEnd drives a real OS-produced descriptor
// through registration to verify the decode and socket recreation succeed and
// the session advances to StateSocketReady.
func TestRegisterDuplicateSocket_EndToEnd(t *testing.T) {
	c := New()
	c.sessionID = "s"
	c.state = StateSourceExported

	if err := c.RegisterDuplicateSocket(context.Background(), "s", connectedSocketDescriptor(t)); err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	t.Cleanup(func() {
		if c.dupSocket != 0 {
			_ = windows.Closesocket(c.dupSocket)
		}
	})

	if got := c.State(); got != StateSocketReady {
		t.Fatalf("state = %s, want %s", got, StateSocketReady)
	}
	if c.dupSocket == 0 {
		t.Fatal("dupSocket not set")
	}
	select {
	case <-c.socketReady:
	default:
		t.Fatal("socketReady not closed")
	}
}
