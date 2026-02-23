//go:build windows

/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package shim

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	winio "github.com/Microsoft/go-winio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/log"
	"github.com/containerd/ttrpc"
	"golang.org/x/sys/windows"
)

// notifyReady signals the parent process that the shim server is ready.
// On Windows, this closes stdout and sets a named event that the parent
// process is waiting on to know the shim has started successfully.
func notifyReady(_ context.Context, serrs chan error) error {
	select {
	case err := <-serrs:
		return err
	case <-time.After(2 * time.Millisecond):
		// This is our best indication that we have not errored on creation
		// and are successfully serving the API.
		os.Stdout.Close()
		eventName, _ := windows.UTF16PtrFromString(fmt.Sprintf("%s-%s", namespaceFlag, id))
		// Open the existing event and set it to wake up the parent process which is waiting for the shim to be ready.
		handle, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, eventName)
		if err == nil {
			_ = windows.SetEvent(handle)    // Wake up the parent
			_ = windows.CloseHandle(handle) // Clean up
		}
	}
	return nil
}

// setupSignals creates a signal channel for Windows.
// On Windows, we don't register any signals here because:
// 1. Child process reaping (SIGCHLD) is not needed - the OS handles it.
// 2. Exit signals (SIGINT/SIGTERM) are handled by handleExitSignals separately.
// We return an empty channel that reap() can use, but it won't receive signals.
func setupSignals(_ Config) (chan os.Signal, error) {
	signals := make(chan os.Signal, 32)
	return signals, nil
}

// newServer creates a new ttrpc server for Windows.
// Unlike Unix, Windows doesn't have user-based socket authentication,
// so we create a basic ttrpc server without the handshaker.
func newServer(opts ...ttrpc.ServerOpt) (*ttrpc.Server, error) {
	return ttrpc.NewServer(opts...)
}

// subreaper is not applicable on Windows as the OS automatically
// handles orphaned processes differently than Unix systems.
func subreaper() error {
	// This is a no-op on Windows - the OS handles orphaned processes
	return nil
}

// setupDumpStacks is currently not implemented for Windows.
// Windows doesn't have SIGUSR1, so stack dumping would need to use
// a different mechanism (e.g., a named event or debug console).
func setupDumpStacks(_ chan<- os.Signal) {
	// No-op on Windows - SIGUSR1 doesn't exist
	// Future: could implement using Windows events or console signals
}

// serveListener creates a named pipe listener for Windows.
// If path is provided, it creates a new named pipe at that location.
// If path is empty and fd is provided, it attempts to inherit the listener (not commonly used on Windows).
func serveListener(path string, _ uintptr) (net.Listener, error) {
	if path == "" {
		// On Windows, inheriting file descriptors is more complex and rarely used
		// with named pipes. We'll return an error if no path is provided.
		return nil, fmt.Errorf("named pipe path is required on Windows")
	}

	// Ensure the path is in the correct Windows named pipe format
	// Expected format: \\.\pipe\<name>
	if !strings.HasPrefix(path, `\\.\pipe`) {
		return nil, fmt.Errorf("socket is required to be pipe address")
	}

	l, err := winio.ListenPipe(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create named pipe listener at %s: %w", path, err)
	}

	log.L.WithField("pipe", path).Debug("serving api on named pipe")
	return l, nil
}

// reap handles signals on Windows. Unlike Unix, Windows doesn't send SIGCHLD
// when child processes exit, so we only need to handle shutdown signals.
func reap(ctx context.Context, logger *log.Entry, signals chan os.Signal) error {
	logger.Debug("starting signal loop")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case s := <-signals:
			logger.WithField("signal", s).Debug("received signal in reap loop")
			// On Windows, we just log the signal
			// Exit signals are handled in handleExitSignals
		}
	}
}

// handleExitSignals listens for shutdown signals (SIGINT, SIGTERM) and
// triggers the provided cancel function for graceful shutdown.
func handleExitSignals(ctx context.Context, logger *log.Entry, cancel context.CancelFunc) {
	ch := make(chan os.Signal, 32)
	// On Windows, os.Kill cannot be caught. We handle os.Interrupt (Ctrl+C) and SIGTERM.
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case s := <-ch:
			logger.WithField("signal", s).Debug("caught exit signal")
			cancel()
			return
		case <-ctx.Done():
			return
		}
	}
}

// openLog creates a named pipe for shim logging on Windows.
// The containerd daemon connects to this pipe as a client to read log output.
// The pipe format is: \\.\pipe\containerd-shim-{namespace}-{id}-log
func openLog(ctx context.Context, id string) (io.Writer, error) {
	ns, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return nil, err
	}
	pipePath := fmt.Sprintf("\\\\.\\pipe\\containerd-shim-%s-%s-log", ns, id)
	l, err := winio.ListenPipe(pipePath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create shim log pipe: %w", err)
	}

	rlw := &reconnectingLogWriter{
		l: l,
	}

	// Accept connections from containerd in the background.
	// Supports reconnection if containerd restarts.
	go rlw.acceptConnections()

	return rlw, nil
}

// reconnectingLogWriter is a writer that accepts log connections from containerd.
// It supports reconnection - if containerd restarts, a new connection is accepted
// and the old one is closed. Logs generated during reconnection may be lost.
type reconnectingLogWriter struct {
	l    net.Listener // The named pipe listener waiting for connections
	mu   sync.Mutex   // Protects the current connection
	conn net.Conn     // The current active connection (may be nil)
}

// acceptConnections listens for log connections in the background.
func (rlw *reconnectingLogWriter) acceptConnections() {
	for {
		newConn, err := rlw.l.Accept()
		if err != nil {
			// Listener was closed, stop accepting
			return
		}

		rlw.mu.Lock()
		// Close the old connection if one exists
		if rlw.conn != nil {
			rlw.conn.Close()
		}
		rlw.conn = newConn
		rlw.mu.Unlock()
	}
}

// Write implements io.Writer. It writes to the current connection if one exists.
// If no connection is established yet, writes are silently dropped to avoid
// blocking the shim.
func (rlw *reconnectingLogWriter) Write(p []byte) (n int, err error) {
	rlw.mu.Lock()
	conn := rlw.conn
	rlw.mu.Unlock()

	if conn == nil {
		// No connection yet, drop the log.
		return len(p), nil
	}

	n, err = conn.Write(p)
	if err != nil {
		// Connection may have been closed, clear it so next write
		// doesn't try to use a broken connection
		rlw.mu.Lock()
		if rlw.conn == conn {
			rlw.conn.Close()
			rlw.conn = nil
		}
		rlw.mu.Unlock()
		// Return success anyway to avoid log write errors propagating
		return len(p), nil
	}
	return n, nil
}

// Close implements io.Closer. It closes both the listener and any active connection.
func (rlw *reconnectingLogWriter) Close() error {
	rlw.mu.Lock()
	defer rlw.mu.Unlock()

	var err error
	if rlw.l != nil {
		err = rlw.l.Close()
	}
	if rlw.conn != nil {
		if cerr := rlw.conn.Close(); cerr != nil && err == nil {
			err = cerr
		}
		rlw.conn = nil
	}
	return err
}
