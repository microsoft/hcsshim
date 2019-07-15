package uvm

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
)

// Wait waits synchronously for a utility VM to terminate.
func (uvm *UtilityVM) Wait(ctx context.Context) error {
	err := uvm.hcsSystem.Wait()

	// outputProcessingCancel will only cancel waiting for the vsockexec
	// connection, it won't stop output processing once the connection is
	// established.
	if uvm.outputProcessingCancel != nil {
		uvm.outputProcessingCancel()
	}
	log.G(ctx).WithField(logfields.UVMID, uvm.id).Debug("uvm exited, waiting for output processing to complete")
	if uvm.outputProcessingDone != nil {
		<-uvm.outputProcessingDone
	}

	return err
}
