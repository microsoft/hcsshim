package hcsv2

import (
	"strings"

	oci "github.com/opencontainers/runtime-spec/specs-go"
)

// getNetworkNamespaceID returns the `ToLower` of
// `spec.Windows.Network.NetworkNamespace` or `""`.
func getNetworkNamespaceID(spec *oci.Spec) string {
	if spec.Windows != nil &&
		spec.Windows.Network != nil {
		return strings.ToLower(spec.Windows.Network.NetworkNamespace)
	}
	return ""
}

// isRootReadonly returns `true` if the spec specifies the rootfs is readonly.
func isRootReadonly(spec *oci.Spec) bool {
	if spec.Root != nil {
		return spec.Root.Readonly
	}
	return false
}

// isInMounts returns `true` if `target` matches a `Destination` in any of
// `mounts`.
func isInMounts(target string, mounts []oci.Mount) bool {
	for _, m := range mounts {
		if m.Destination == target {
			return true
		}
	}
	return false
}
