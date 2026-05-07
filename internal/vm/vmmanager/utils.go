//go:build windows && (lcow || wcow)

package vmmanager

import (
	"context"
	"net"
)

// AcceptConnection accepts a connection and then closes a listener.
// It monitors ctx.Done() and uvm.Wait() for early termination.
func (uvm *UtilityVM) AcceptConnection(ctx context.Context, l net.Listener, closeConnection bool) (net.Conn, error) {
	// Channel to capture the accept result
	type acceptResult struct {
		conn net.Conn
		err  error
	}
	resultCh := make(chan acceptResult, 1)

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
		_ = l.Close()
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
