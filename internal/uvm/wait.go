package uvm

import (
	"github.com/microsoft/hcsshim/internal/logfields"
	"github.com/sirupsen/logrus"
)

func (uvm *UtilityVM) waitForOutput() {
	logrus.WithField(logfields.UVMID, uvm.id).
		Debug("UVM exited, waiting for output processing to complete")
	if uvm.outputProcessingDone != nil {
		<-uvm.outputProcessingDone
	}
}

// Wait waits synchronously for a utility VM to terminate.
func (uvm *UtilityVM) Wait() error {
	err := uvm.hcsSystem.Wait()

	// outputProcessingCancel will only cancel waiting for the vsockexec
	// connection, it won't stop output processing once the connection is
	// established.
	if uvm.outputProcessingCancel != nil {
		uvm.outputProcessingCancel()
	}
	uvm.waitForOutput()

	return err
}
