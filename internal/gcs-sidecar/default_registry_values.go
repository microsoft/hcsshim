//go:build windows
// +build windows

package bridge

import (
	"math"
	"reflect"
	"slices"
	"strconv"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// DefaultRegistryValues contains the registry values that are always allowed
// without requiring policy validation. These are common system settings needed
// for proper UVM operation.
var defaultRegistryValues = []hcsschema.RegistryValue{
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
	return slices.ContainsFunc(defaultRegistryValues, func(rv hcsschema.RegistryValue) bool {
		return registryValuesMatch(rv, value)
	})
}

// registryValuesMatch checks if two registry values are equivalent.
// Assumes registry values are well-formed (only relevant value fields are populated for each Type_).
func registryValuesMatch(a, b hcsschema.RegistryValue) bool {
	return reflect.DeepEqual(a, b)
}
