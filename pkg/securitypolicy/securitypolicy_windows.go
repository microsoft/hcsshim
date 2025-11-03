//go:build windows
// +build windows

package securitypolicy

import oci "github.com/opencontainers/runtime-spec/specs-go"

//nolint:unused
const osType = "windows"

// SandboxMountsDir returns sandbox mounts directory inside UVM/host.
func SandboxMountsDir(sandboxID string) string {
	return ""
}

// HugePagesMountsDir returns hugepages mounts directory inside UVM.
func HugePagesMountsDir(sandboxID string) string {
	return ""
}

func GetAllUserInfo(process *oci.Process, rootPath string) (IDName, []IDName, string, error) {
	return IDName{}, []IDName{}, "", nil
}

// DefaultCRIMounts returns default mounts added to windows spec by containerD.
func DefaultCRIMounts() []oci.Mount {
	return []oci.Mount{}
}

// DefaultCRIPrivilegedMounts returns a slice of mounts which are added to the
// windows container spec when a container runs in a privileged mode.
func DefaultCRIPrivilegedMounts() []oci.Mount {
	return []oci.Mount{}
}
