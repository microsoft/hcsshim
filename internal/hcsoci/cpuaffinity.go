//go:build windows
// +build windows

package hcsoci

import (
	"errors"
	"fmt"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/jobobject"
	"github.com/Microsoft/hcsshim/osversion"
)

// Shared, container-kind-agnostic CPU affinity helpers. These are used by every
// Windows container shape that honors spec.Windows.Resources.CPU.Affinity:
// HostProcess (internal/jobcontainers) and Argon (this package). Keeping them
// here, rather than in a kind-specific file, avoids duplicating the validation
// and conversion logic across packages.

// Sentinel errors returned by ValidateCPUAffinity / ValidateCPUAffinityEntries.
var (
	// ErrCPUAffinityMultipleGroupsNotSupported is returned when multiple processor-group
	// affinity entries are requested on a host older than Windows Server 2022 (build 20348),
	// which does not support multi-group affinity for job object silos.
	// On Windows Server 2022+, multiple processor groups are fully supported.
	ErrCPUAffinityMultipleGroupsNotSupported = errors.New("cpu affinity with multiple processor groups requires Windows Server 2022 or later")
	// ErrCPUAffinityNonZeroGroupNotSupported is returned when a non-zero processor group is
	// requested on a host older than Windows Server 2022 (build 20348).
	// On Windows Server 2022+, non-zero processor groups are fully supported.
	ErrCPUAffinityNonZeroGroupNotSupported = errors.New("cpu affinity with a non-zero processor group requires Windows Server 2022 or later")
	// ErrCPUAffinityMaskZero is returned when an affinity entry has a zero bitmask,
	// which would select no processors and is always invalid.
	ErrCPUAffinityMaskZero = errors.New("cpu affinity mask must be non-zero")
)

// ValidateCPUAffinity handles the logic of validating the container's CPU affinity
// specified in the OCI spec.
//
// Returns the validated affinity entries (nil if not specified) and any validation error.
// Multiple processor groups and non-zero group numbers require Windows Server 2022
// (build 20348) or later; on older hosts only a single entry for group 0 is accepted.
func ValidateCPUAffinity(spec *specs.Spec) ([]specs.WindowsCPUGroupAffinity, error) {
	if spec.Windows == nil || spec.Windows.Resources == nil || spec.Windows.Resources.CPU == nil {
		return nil, nil
	}
	return ValidateCPUAffinityEntries(spec.Windows.Resources.CPU.Affinity)
}

// ValidateCPUAffinityEntries validates a set of OCI CPU affinity entries directly,
// applying the same rules as ValidateCPUAffinity. It is used on the container update
// path, where the affinity is supplied as a bare slice rather than a full spec.
//
// Returns the validated entries (nil if empty) and any validation error.
func ValidateCPUAffinityEntries(affinity []specs.WindowsCPUGroupAffinity) ([]specs.WindowsCPUGroupAffinity, error) {
	if len(affinity) == 0 {
		return nil, nil
	}

	// Zero masks are never valid regardless of OS version.
	for i, a := range affinity {
		if a.Mask == 0 {
			return nil, fmt.Errorf("%w: entry %d has zero mask", ErrCPUAffinityMaskZero, i)
		}
	}

	// Determine whether multi-group features are needed: either multiple entries,
	// or a single entry targeting a non-zero processor group.
	multiGroup := len(affinity) > 1 || affinity[0].Group != 0

	// Multiple processor groups are only supported on Windows Server 2022+.
	if multiGroup && osversion.Build() < osversion.LTSC2022 {
		if len(affinity) > 1 {
			return nil, fmt.Errorf("%w: %d entries", ErrCPUAffinityMultipleGroupsNotSupported, len(affinity))
		}
		return nil, fmt.Errorf("%w: group %d", ErrCPUAffinityNonZeroGroupNotSupported, affinity[0].Group)
	}

	return affinity, nil
}

// ToJobObjectAffinities converts validated OCI CPU affinity entries into the
// jobobject.GroupAffinity representation used by the Win32 job-object APIs.
//
// The input is expected to already have been run through ValidateCPUAffinity.
func ToJobObjectAffinities(affinities []specs.WindowsCPUGroupAffinity) []jobobject.GroupAffinity {
	if len(affinities) == 0 {
		return nil
	}
	out := make([]jobobject.GroupAffinity, len(affinities))
	for i, a := range affinities {
		out[i] = jobobject.GroupAffinity{
			Mask:  a.Mask,
			Group: uint16(a.Group),
		}
	}
	return out
}
