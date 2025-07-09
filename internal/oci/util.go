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
func IsJobContainer(s *specs.Spec) bool {
	return IsWCOW(s) && s.Annotations[annotations.HostProcessContainer] == "true"
}
