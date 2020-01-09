package oci

import (
	"fmt"
	"strings"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
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

// ValidateSupportedMounts validates that `s` contains no mounts using features
// explicitly disabled in `opts`.
func ValidateSupportedMounts(s specs.Spec, opts *runhcsopts.Options) error {
	if opts == nil {
		return nil
	}

	const errorfmt = "Unsupported mount source: %q, type: %q"
	invalid := false
	for _, m := range s.Mounts {
		switch m.Type {
		case "", "bind":
			invalid = (strings.HasPrefix(m.Source, "sandbox://") && opts.DisableSandboxMounts) ||
				(strings.HasPrefix(m.Source, `\\.\pipe`) && opts.DisablePipeMounts) ||
				opts.DisableBindMounts
		case "physical-disk":
			invalid = opts.DisablePhysicalDiskMounts
		case "virtual-disk", "automanage-virtual-disk":
			invalid = opts.DisableVirtualDiskMounts
		}
		if invalid {
			return fmt.Errorf(errorfmt, m.Source, m.Type)
		}
	}
	return nil
}
