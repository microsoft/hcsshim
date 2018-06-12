package uvm

import (
	"github.com/Microsoft/hcsshim/internal/schema2"
)

// Test whether the Guest is RS5 or not
func (uvm *UtilityVM) IsRS5() bool {
	if uvm.supportedSchema == nil {
		properties, err := uvm.hcsSystem.PropertiesV2(schema2.PropertyTypeGuestInterface)
		if err != nil {
			return false
		}
		uvm.supportedSchema = properties.GuestInterfaceInfo.SupportedSchemaVersions
	}

	for _, schema := range uvm.supportedSchema {
		if schema.Major > 2 || (schema.Major == 2 && schema.Minor > 0) {
			return true
		}
	}

	return false
}
