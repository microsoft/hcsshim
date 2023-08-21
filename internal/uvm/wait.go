//go:build windows

package uvm

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/logfields"
)

// Wait waits synchronously for a utility VM to terminate.
func (uvm *UtilityVM) Wait() error { return uvm.WaitCtx(context.Background()) }

// Wait waits synchronously for a utility VM to terminate, or the context to be cancelled.
func (uvm *UtilityVM) WaitCtx(ctx context.Context) (err error) {
	err = uvm.hcsSystem.WaitCtx(ctx)

	e := logrus.WithField(logfields.UVMID, uvm.id)
	e.Debug("uvm exited, waiting for output processing to complete")
	if uvm.outputProcessingDone != nil {
		select {
		case <-uvm.outputProcessingDone:
		case <-ctx.Done():
			if err2 := ctx.Err(); err2 != nil {
				e.WithError(err2).Warning("failed to wait on uvm output processing")
				if err == nil {
					err = fmt.Errorf("failed to wait on uvm output processing: %w", err2)
				}
			}
		}
	}

	return err
}
