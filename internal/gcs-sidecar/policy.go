//go:build windows
// +build windows

package bridge

import (
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

// DefaultCRIMounts returns default mounts added to windows spec by containerD.
func DefaultCRIMounts() []oci.Mount {
	return []oci.Mount{}
}

// DefaultCRIPrivilegedMounts returns a slice of mounts which are added to the
// windows container spec when a container runs in a privileged mode.
func DefaultCRIPrivilegedMounts() []oci.Mount {
	return []oci.Mount{}
}
