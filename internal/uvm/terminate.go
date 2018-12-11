package uvm

import (
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/sirupsen/logrus"
)

// Terminate requests a utility VM terminate. If IsPending() on the error returned is true,
// it may not actually be shut down until Wait() succeeds.
func (uvm *UtilityVM) Terminate() (err error) {
	op := "uvm::Terminate"
	log := logrus.WithFields(logrus.Fields{
		logfields.UVMID: uvm.id,
	})
	log.Debug(op + " - Begin Operation")
	defer func() {
		if err != nil {
			log.Data[logrus.ErrorKey] = err
			log.Error(op + " - End Operation - Error")
		} else {
			log.Debug(op + " - End Operation - Success")
		}
	}()

	return uvm.hcsSystem.Terminate()
}
