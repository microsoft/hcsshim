package uvm

import (
	"fmt"
)

// Terminate requests a utility VM terminate. If IsPending() on the error returned is true,
// it may not actually be shut down until Wait() succeeds.
func (uvm *UtilityVM) Terminate() error {
	if uvm.hcsSystem == nil {
		return fmt.Errorf("cannot terminate - no hcsSystem handle")
	}
	return convertSystemError(uvm.hcsSystem.Terminate(), uvm)
}
