package uvm

import (
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/sirupsen/logrus"
)

// Wait waits synchronously for a utility VM to terminate.
func (uvm *UtilityVM) Wait() error {
	err := uvm.hcsSystem.Wait()

	// outputProcessingCancel will only cancel waiting for the vsockexec
	// connection, it won't stop output processing once the connection is
	// established.
	if uvm.outputProcessingCancel != nil {
		uvm.outputProcessingCancel()
	}
	logrus.WithField(logfields.UVMID, uvm.id).Debug("UVM exited, waiting for output processing to complete")
	if uvm.outputProcessingDone != nil {
		<-uvm.outputProcessingDone
	}

	return err
}
