//go:build windows

package guestmanager

import (
	"context"
	"net"

	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"
)

// AcceptConnection accepts a connection and then closes a listener.
// It monitors ctx.Done() and uvm.Wait() for early termination.
func AcceptConnection(ctx context.Context, uvm vmmanager.LifetimeManager, l net.Listener, closeConnection bool) (net.Conn, error) {
	// Channel to capture the accept result
	type acceptResult struct {
		conn net.Conn
		err  error
	}
	resultCh := make(chan acceptResult)

	go func() {
		c, err := l.Accept()
		resultCh <- acceptResult{c, err}
	}()

	// Channel to monitor VM exit
	vmExitCh := make(chan error, 1)
	go func() {
		// Wait blocks until the VM terminates
		vmExitCh <- uvm.Wait(ctx)
	}()

	select {
	case res := <-resultCh:
		if closeConnection {
			_ = l.Close()
		}
		return res.conn, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-vmExitCh:
	}

	_ = l.Close()
	res := <-resultCh
	if res.err == nil {
		return res.conn, res.err
	}

	// Prefer context error to VM error to accept error in order to return the
	// most useful error.
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if uvm.ExitError() != nil {
		return nil, uvm.ExitError()
	}
	return nil, res.err
}
