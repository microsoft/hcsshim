//go:build linux
// +build linux

// Package spec encapsulates a number of GCS specific oci spec modifications, e.g.,
// networking mounts, sandbox path substitutions in guest etc.
//
// TODO: consider moving oci spec specific code from /internal/guest/runtime/hcsv2/spec.go
package spec

import (
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

// networkingMountPaths returns an array of mount paths to enable networking
// inside containers.
func networkingMountPaths() []string {
	return []string{
		"/etc/hostname",
		"/etc/hosts",
		"/etc/resolv.conf",
	}
}

// GenerateWorkloadContainerNetworkMounts generates an array of specs.Mount
// required for container networking. Original spec is left untouched and
// it's the responsibility of a caller to update it.
func GenerateWorkloadContainerNetworkMounts(sandboxID string, spec *oci.Spec) []oci.Mount {
	var nMounts []oci.Mount

	for _, mountPath := range networkingMountPaths() {
		// Don't override if the mount is present in the spec
		if MountPresent(mountPath, spec.Mounts) {
			continue
		}
		options := []string{"bind"}
		if spec.Root != nil && spec.Root.Readonly {
			options = append(options, "ro")
		}
		trimmedMountPath := strings.TrimPrefix(mountPath, "/etc/")
		mt := oci.Mount{
			Destination: mountPath,
			Type:        "bind",
			Source:      filepath.Join(SandboxRootDir(sandboxID), trimmedMountPath),
			Options:     options,
		}
		nMounts = append(nMounts, mt)
	}
	return nMounts
}

// MountPresent checks if mountPath is present in the specMounts array.
func MountPresent(mountPath string, specMounts []oci.Mount) bool {
	for _, m := range specMounts {
		if m.Destination == mountPath {
			return true
		}
	}
	return false
}

// SandboxRootDir returns the sandbox container root directory inside UVM/host.
func SandboxRootDir(sandboxID string) string {
	return filepath.Join(guestpath.LCOWRootPrefixInUVM, sandboxID)
}

// SandboxMountsDir returns sandbox mounts directory inside UVM/host.
func SandboxMountsDir(sandboxID string) string {
	return filepath.Join(SandboxRootDir(sandboxID), "sandboxMounts")
}

// HugePagesMountsDir returns hugepages mounts directory inside UVM.
func HugePagesMountsDir(sandboxID string) string {
	return filepath.Join(SandboxRootDir(sandboxID), "hugepages")
}

// SandboxMountSource returns sandbox mount path inside UVM
func SandboxMountSource(sandboxID, path string) string {
	mountsDir := SandboxMountsDir(sandboxID)
	subPath := strings.TrimPrefix(path, guestpath.SandboxMountPrefix)
	return filepath.Join(mountsDir, subPath)
}

// HugePagesMountSource returns hugepages mount path inside UVM
func HugePagesMountSource(sandboxID, path string) string {
	mountsDir := HugePagesMountsDir(sandboxID)
	subPath := strings.TrimPrefix(path, guestpath.HugePagesMountPrefix)
	return filepath.Join(mountsDir, subPath)
}
