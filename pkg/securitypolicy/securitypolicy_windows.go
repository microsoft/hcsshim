//go:build windows
// +build windows

package securitypolicy

import (
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

//nolint:unused
const osType = "windows"

// validateHostData fetches SNP report (if applicable) and validates `hostData` against
// HostData set at UVM launch.
func validateHostData(hostData []byte) error {
	if err := GetPspDriverError(); err != nil {
		// For this case gcs-sidecar will keep initial deny policy.
		return errors.Wrapf(err, "an error occurred while using PSP driver")
	}

	if err := ValidateHostDataPSP(hostData[:]); err != nil {
		// For this case gcs-sidecar will keep initial deny policy.
		return err
	}
	return nil
}

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
