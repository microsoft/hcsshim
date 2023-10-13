package hcsschema

import "strings"

func NewSystemType(x string) (SystemType, error) {
	return enumLookup(map[string]SystemType{
		strings.ToLower(string(SystemType_CONTAINER)):       SystemType_CONTAINER,
		strings.ToLower(string(SystemType_VIRTUAL_MACHINE)): SystemType_VIRTUAL_MACHINE,
	}, strings.ToLower(x))
}
