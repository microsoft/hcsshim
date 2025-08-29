//go:build windows
// +build windows

package securitypolicy

import oci "github.com/opencontainers/runtime-spec/specs-go"

// This is being used by StandEnforcer and is a no-op for windows.
// substituteUVMPath substitutes mount prefix to an appropriate path inside
// UVM. At policy generation time, it's impossible to tell what the sandboxID
// will be, so the prefix substitution needs to happen during runtime.
func substituteUVMPath(sandboxID string, m mountInternal) mountInternal {
	//no-op for windows
	_ = sandboxID
	return m
}

// SandboxMountsDir returns sandbox mounts directory inside UVM/host.
func SandboxMountsDir(sandboxID string) string {
	return ""
}

// HugePagesMountsDir returns hugepages mounts directory inside UVM.
func HugePagesMountsDir(sandboxID string) string {
	return ""
}

func GetAllUserInfo(containerID string, process *oci.Process) (IDName, []IDName, string, error) {
	return IDName{}, []IDName{}, "", nil
}
