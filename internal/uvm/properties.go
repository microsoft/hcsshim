package uvm

import (
	"github.com/Microsoft/hcsshim/internal/schema2"
)

// Get properties for the utility vm
func (uvm *UtilityVM) Properties(types ...schema2.PropertyType) (*schema2.SystemProperties, error) {
	return uvm.hcsSystem.PropertiesV2(types...)
}
