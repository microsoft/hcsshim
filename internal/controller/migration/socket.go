//go:build windows && lcow

package migration

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/containerd/errdefs"
	"golang.org/x/sys/windows"
)

// wsaVersion is the Winsock version requested by [ensureWinsock].
const wsaVersion uint32 = 0x0202

// ensureWinsock initializes Winsock once for the process so a socket can be
// recreated without depending on other code having initialized it first.
// WSACleanup is deliberately never called, so Winsock stays up for the
// process lifetime.
var ensureWinsock = sync.OnceValue(func() error {
	var data windows.WSAData
	if err := windows.WSAStartup(wsaVersion, &data); err != nil {
		return fmt.Errorf("WSAStartup: %w", err)
	}

	return nil
})

// soConnectTime is the SO_CONNECT_TIME socket option. Querying it with
// getsockopt yields how many seconds the socket has been connected, or
// 0xFFFFFFFF if it has never connected. It is not exported by
// [golang.org/x/sys/windows].
const soConnectTime int32 = 0x700C

// connectTimeNotConnected is the SO_CONNECT_TIME sentinel returned for an
// unconnected socket.
const connectTimeNotConnected uint32 = 0xFFFFFFFF

// RegisterDuplicateSocket adopts a duplicated migration transport socket,
// described by protocolInfo, into this process and makes it available to the
// pending transfer for the given session. A repeat call for an already-adopted
// session is a no-op.
func (c *Controller) RegisterDuplicateSocket(ctx context.Context, sessionID string, protocolInfo []byte) error {
	// Reject input too small to hold a serialized socket descriptor before
	// attempting to decode it.
	wantSize := int(unsafe.Sizeof(windows.WSAProtocolInfo{}))
	if len(protocolInfo) < wantSize {
		return fmt.Errorf("protocol info is %d bytes, want at least %d: %w", len(protocolInfo), wantSize, errdefs.ErrInvalidArgument)
	}

	// Decode the opaque caller-supplied bytes into the socket descriptor
	// used to recreate the duplicated socket.
	var info windows.WSAProtocolInfo
	if err := binary.Read(bytes.NewReader(protocolInfo), binary.LittleEndian, &info); err != nil {
		return fmt.Errorf("decode WSAProtocolInfo: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sessionID != sessionID {
		return fmt.Errorf("session id %q does not match active session %q: %w", sessionID, c.sessionID, errdefs.ErrInvalidArgument)
	}

	// Idempotent: a repeat call for the same session is a no-op once the
	// socket has been adopted.
	if c.dupSocket != 0 {
		return nil
	}

	// Transfer may have already claimed the session (StateSocketWaiting) and
	// be waiting on the socket; allow registration in that case too.
	if c.state != StateSourceExported && c.state != StateDestinationPrepared && c.state != StateSocketWaiting {
		return fmt.Errorf("register duplicate socket requires state %s or %s (current: %s): %w", StateSourceExported, StateDestinationPrepared, c.state, errdefs.ErrFailedPrecondition)
	}

	// Make sure Winsock is up for this process before recreating the socket.
	if err := ensureWinsock(); err != nil {
		return err
	}

	// Recreate the duplicated socket in this process from the descriptor so
	// the transfer can use it as its transport.
	sock, err := windows.WSASocket(info.AddressFamily, info.SocketType, info.Protocol, &info, 0, 0)
	if err != nil {
		return fmt.Errorf("WSASocket: %w", err)
	}

	// Verify the duplicated handle actually represents a connected socket;
	// HCS will fail the migration if we hand it an unconnected endpoint.
	var connectTime uint32
	optLen := int32(unsafe.Sizeof(connectTime))
	if err := windows.Getsockopt(sock, windows.SOL_SOCKET, soConnectTime, (*byte)(unsafe.Pointer(&connectTime)), &optLen); err != nil {
		_ = windows.Closesocket(sock)
		return fmt.Errorf("getsockopt SO_CONNECT_TIME: %w", err)
	}

	if connectTime == connectTimeNotConnected {
		_ = windows.Closesocket(sock)
		return fmt.Errorf("duplicated socket is not connected: %w", errdefs.ErrFailedPrecondition)
	}

	c.dupSocket = sock
	c.state = StateSocketReady
	close(c.socketReady)

	log.G(ctx).Info("duplicate migration socket registered")
	return nil
}
