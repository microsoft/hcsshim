package oci

import "github.com/opencontainers/runtime-spec/specs-go"

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

// IsPrivileged checks if `s` is asking for a Windows privileged container.
func IsPrivileged(s *specs.Spec) bool {
	if _, ok := s.Annotations[annotationPrivileged]; ok {
		return true
	}
	return false
}
