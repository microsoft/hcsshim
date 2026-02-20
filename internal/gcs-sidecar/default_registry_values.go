//go:build windows
// +build windows

package bridge

import (
	"math"
	"strconv"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// DefaultRegistryValues contains the registry values that are always allowed
// without requiring policy validation. These are common system settings needed
// for proper UVM operation.
var DefaultRegistryValues = []hcsschema.RegistryValue{
	{
		Key: &hcsschema.RegistryKey{
			Hive: hcsschema.RegistryHive_SYSTEM,
			Name: "ControlSet001\\Control",
		},
		Name:        "WaitToKillServiceTimeout",
		StringValue: strconv.Itoa(math.MaxInt32),
		Type_:       hcsschema.RegistryValueType_STRING,
	},
}

// isDefaultRegistryValue checks if the given registry value matches one of the default allowed values
func isDefaultRegistryValue(value hcsschema.RegistryValue) bool {
	for _, defaultVal := range DefaultRegistryValues {
		if registryValuesMatch(defaultVal, value) {
			return true
		}
	}
	return false
}

// registryValuesMatch checks if two registry values are equivalent
func registryValuesMatch(a, b hcsschema.RegistryValue) bool {
	// Check if keys match
	if !registryKeysMatch(a.Key, b.Key) {
		return false
	}

	// Check if names match
	if a.Name != b.Name {
		return false
	}

	// Check if types match
	if a.Type_ != b.Type_ {
		return false
	}

	// Check type-specific values
	switch a.Type_ {
	case hcsschema.RegistryValueType_STRING:
		return a.StringValue == b.StringValue
	case hcsschema.RegistryValueType_EXPANDED_STRING:
		return a.StringValue == b.StringValue
	case hcsschema.RegistryValueType_MULTI_STRING:
		return a.StringValue == b.StringValue
	case hcsschema.RegistryValueType_D_WORD:
		return a.DWordValue == b.DWordValue
	case hcsschema.RegistryValueType_Q_WORD:
		return a.QWordValue == b.QWordValue
	case hcsschema.RegistryValueType_BINARY:
		return a.BinaryValue == b.BinaryValue
	case hcsschema.RegistryValueType_CUSTOM_TYPE:
		// For CustomType, both CustomType field and BinaryValue must match
		return a.CustomType == b.CustomType && a.BinaryValue == b.BinaryValue
	case hcsschema.RegistryValueType_NONE:
		// NONE type has no value to compare
		return true
	default:
		return false
	}
}

// registryKeysMatch checks if two registry keys are equivalent
func registryKeysMatch(a, b *hcsschema.RegistryKey) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Hive == b.Hive && a.Name == b.Name && a.Volatile == b.Volatile
}
