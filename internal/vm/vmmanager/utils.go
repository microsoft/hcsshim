//go:build windows

package vmmanager

import (
	"context"
	"net"
)

// vmWaiter exposes the subset of VM lifecycle needed by [AcceptConnection]:
// Implemented by [UtilityVM].
type vmWaiter interface {
	// Wait blocks until the VM exits or ctx is cancelled.
	Wait(ctx context.Context) error
	// ExitError returns the error that caused the VM to exit, if any.
	ExitError() error
}

// AcceptConnection accepts a connection and then closes a listener.
// It monitors ctx.Done() and uvm.Wait() for early termination.
func AcceptConnection(ctx context.Context, uvm vmWaiter, l net.Listener, closeConnection bool) (net.Conn, error) {
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

	// Prefer context error to VM error to accept error in order to return the
	// most useful error.
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return nil, uvm.ExitError()
}
