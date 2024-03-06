//go:build windows

package uvm

import (
	"context"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/logfields"
)

// Wait waits synchronously for a utility VM to terminate.
func (uvm *UtilityVM) Wait() error { return uvm.WaitCtx(context.Background()) }

// Wait waits synchronously for a utility VM to terminate, or the context to be cancelled.
func (uvm *UtilityVM) WaitCtx(ctx context.Context) (err error) {
	err = uvm.hcsSystem.WaitCtx(ctx)

	logrus.WithField(logfields.UVMID, uvm.id).Debug("uvm exited, waiting for output processing to complete")
	var outputErr error
	if uvm.outputProcessingDone != nil {
		select {
		case <-uvm.outputProcessingDone:
		case <-ctx.Done():
			outputErr = fmt.Errorf("failed to wait on uvm output processing: %w", ctx.Err())
		}
	}

	return errors.Join(err, outputErr)
}
