package uvm

import (
	"fmt"
)

// Waits synchronously waits for a utility VM to terminate.
func (uvm *UtilityVM) Wait() error {
	if uvm.hcsSystem == nil {
		return fmt.Errorf("cannot wait - no hcsSystem handle")
	}
	return convertSystemError(uvm.hcsSystem.Wait(), uvm)
}
