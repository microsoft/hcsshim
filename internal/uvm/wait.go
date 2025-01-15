//go:build windows

package uvm

import (
	"context"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/logfields"
)

// WaitForUvmOrContainerExit waits for the container `c` or its UVM
// to exit. This is used to clean up hcs task and exec resources by
// the caller.
func (uvm *UtilityVM) WaitForUvmOrContainerExit(ctx context.Context, c cow.Container) (err error) {
	select {
	case <-c.WaitChannel():
		return c.WaitError()
	case <-uvm.hcsSystem.WaitChannel():
		logrus.WithField(logfields.UVMID, uvm.id).Debug("uvm exited, waiting for output processing to complete")
		var outputErr error
		if uvm.outputProcessingDone != nil {
			select {
			case <-uvm.outputProcessingDone:
			case <-ctx.Done():
				outputErr = fmt.Errorf("failed to wait on uvm output processing: %w", ctx.Err())
			}
		}
		return errors.Join(uvm.hcsSystem.WaitError(), outputErr)
	case <-ctx.Done():
		return ctx.Err()
	}
}

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
