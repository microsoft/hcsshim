package uvm

import (
	"fmt"
)

// Modifies the compute system by sending a request to HCS
func (uvm *UtilityVM) Modify(hcsModificationDocument interface{}) error {
	if uvm.hcsSystem == nil {
		return fmt.Errorf("cannot modify - no hcsSystem handle")
	}
	return convertSystemError(uvm.hcsSystem.Modify(hcsModificationDocument), uvm)
}
