package uvm

import (
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/sirupsen/logrus"
)

// Waits synchronously waits for a utility VM to terminate.
func (uvm *UtilityVM) Wait() error {
	err := uvm.hcsSystem.Wait()

	logrus.WithField(logfields.ContainerID, uvm.ID).
		Debug("UVM exited, waiting for output processing to complete")
	if uvm.outputProcessingDone != nil {
		<-uvm.outputProcessingDone
	}

	return err
}
