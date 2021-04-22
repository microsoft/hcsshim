package hcs

import "github.com/Microsoft/hcsshim/internal/vm"

func (uvm *utilityVM) Supported(resource vm.Resource, op vm.ResourceOperation) bool {
	// For now at least HCS supports everything we care about.
	return true
}
