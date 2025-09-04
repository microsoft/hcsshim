package oci

import (
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// IsLCOW checks if `s` is a LCOW config.
func IsLCOW(s *specs.Spec) bool {
	return s.Linux != nil
}

// IsWCOW checks if `s` is a WCOW config (argon OR isolated).
func IsWCOW(s *specs.Spec) bool {
	return s.Linux == nil && s.Windows != nil
}

// IsIsolated checks if `s` is hypervisor isolated.
func IsIsolated(s *specs.Spec) bool {
	return IsLCOW(s) || (s.Windows != nil && s.Windows.HyperV != nil)
}

// IsJobContainer checks if `s` is asking for a Windows job container.
// This request is for a shim based process isolated HPCs.
func IsJobContainer(s *specs.Spec) bool {
	return s.Linux == nil &&
		s.Windows != nil &&
		s.Windows.HyperV == nil &&
		s.Annotations[annotations.HostProcessContainer] == "true"
}

// IsIsolatedJobContainer checks if `s` is asking for a Windows job container.
// This request is for running HPC within guest.
func IsIsolatedJobContainer(s *specs.Spec) bool {
	return s.Linux == nil &&
		s.Windows != nil &&
		s.Windows.HyperV != nil &&
		s.Annotations[annotations.HostProcessContainer] == "true"
}
