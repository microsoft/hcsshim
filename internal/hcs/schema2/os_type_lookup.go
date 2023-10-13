package hcsschema

import "strings"

func NewOSType(x string) (OSType, error) {
	return enumLookup(map[string]OSType{
		strings.ToLower(string(OSType_EMPTY)):   OSType_EMPTY,
		strings.ToLower(string(OSType_WINDOWS)): OSType_WINDOWS,
		strings.ToLower(string(OSType_LINUX)):   OSType_LINUX,
	}, strings.ToLower(x))
}
