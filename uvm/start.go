package uvm

import (
	"fmt"
)

// Start synchronously starts the utility VM.
func (uvm *UtilityVM) Start() error {
	if uvm.hcsSystem == nil {
		return fmt.Errorf("cannot start - no hcsSystem handle")
	}
	return convertSystemError(uvm.hcsSystem.Start(), uvm)
}
