//go:build windows && lcow

package migration

import (
	"context"
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
// pending transfer for the given session. A repeat call once a socket is already
// registered is rejected.
func (c *Controller) RegisterDuplicateSocket(ctx context.Context, sessionID string, protocolInfo []byte) (err error) {
	// Reject input too small to hold a serialized socket descriptor before
	// attempting to decode it.
	wantSize := int(unsafe.Sizeof(windows.WSAProtocolInfo{}))
	if len(protocolInfo) < wantSize {
		return fmt.Errorf("protocol info is %d bytes, want at least %d: %w", len(protocolInfo), wantSize, errdefs.ErrInvalidArgument)
	}

	// The bytes are the descriptor's raw native memory as the caller
	// serialized it, so copy them back over the same layout to decode.
	var info windows.WSAProtocolInfo
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&info)), wantSize), protocolInfo)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sessionID != sessionID {
		return fmt.Errorf("session id %q does not match active session %q: %w", sessionID, c.sessionID, errdefs.ErrInvalidArgument)
	}

	// If socket has already been adopted for a session, return an error.
	if c.dupSocket != 0 {
		return fmt.Errorf("duplicate socket already registered for session %q: %w", sessionID, errdefs.ErrAlreadyExists)
	}

	// Transfer may have already claimed the session (StateSocketWaiting) and
	// be waiting on the socket; allow registration in that case too.
	if c.state != StateSourceExported && c.state != StateDestinationPrepared && c.state != StateSocketWaiting {
		return fmt.Errorf("invalid register duplicate socket state (current: %s): %w", c.state, errdefs.ErrFailedPrecondition)
	}

	// sock holds the recreated socket once WSASocket succeeds; until then it
	// stays 0 so the failure cleanup below only closes a socket we own.
	var sock windows.Handle

	// Past the state guard, any failure aborts the session: fail it, drop the
	// socket we created (if any), and close socketReady so a Transfer goroutine
	// blocked on it wakes, observes the failure, and bails instead of
	// hanging until its timeout.
	defer func() {
		if err != nil {
			if sock != 0 {
				_ = windows.Closesocket(sock)
			}

			c.state = StateFailed
			close(c.socketReady)
			log.G(ctx).WithError(err).Error("duplicate migration socket registration failed, session failed")
		}
	}()

	// Make sure Winsock is up for this process before recreating the socket.
	if err = ensureWinsock(); err != nil {
		return err
	}

	// Recreate the duplicated socket in this process from the descriptor so
	// the transfer can use it as its transport.
	s, err := windows.WSASocket(info.AddressFamily, info.SocketType, info.Protocol, &info, 0, 0)
	if err != nil {
		return fmt.Errorf("WSASocket: %w", err)
	}
	sock = s

	// Verify the duplicated handle actually represents a connected socket;
	// HCS will fail the migration if we hand it an unconnected endpoint.
	var connectTime uint32
	optLen := int32(unsafe.Sizeof(connectTime))
	if err = windows.Getsockopt(sock, windows.SOL_SOCKET, soConnectTime, (*byte)(unsafe.Pointer(&connectTime)), &optLen); err != nil {
		return fmt.Errorf("getsockopt SO_CONNECT_TIME: %w", err)
	}

	if connectTime == connectTimeNotConnected {
		return fmt.Errorf("duplicated socket is not connected: %w", errdefs.ErrFailedPrecondition)
	}

	c.dupSocket = sock
	c.state = StateSocketReady
	close(c.socketReady)

	log.G(ctx).Info("duplicate migration socket registered")
	return nil
}
