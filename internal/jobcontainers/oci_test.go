//go:build windows

package jobcontainers

import (
	"context"
	"errors"
	"testing"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/jobobject"
	"github.com/Microsoft/hcsshim/osversion"
)

func TestSpecToLimits_CPUAffinity_Group0MaskSet(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{
			Resources: &specs.WindowsResources{
				CPU: &specs.WindowsCPUResources{
					Affinity: []specs.WindowsCPUGroupAffinity{
						{Mask: 0x3, Group: 0},
					},
				},
			},
		},
	}

	limits, err := specToLimits(context.Background(), "cid", s)
	if err != nil {
		t.Fatalf("specToLimits failed: %v", err)
	}
	if len(limits.GroupAffinities) != 1 ||
		limits.GroupAffinities[0].Mask != 0x3 ||
		limits.GroupAffinities[0].Group != 0 {
		t.Fatalf("unexpected cpu group affinities: got %v", limits.GroupAffinities)
	}
}

func TestSpecToLimits_CPUAffinity_MultiGroup(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{
			Resources: &specs.WindowsResources{
				CPU: &specs.WindowsCPUResources{
					Affinity: []specs.WindowsCPUGroupAffinity{
						{Mask: 0x1, Group: 0},
						{Mask: 0x1, Group: 1},
					},
				},
			},
		},
	}

	limits, err := specToLimits(context.Background(), "cid", s)
	if osversion.Build() >= osversion.LTSC2022 {
		// Multi-group is supported on WS2022+.
		if err != nil {
			t.Fatalf("expected success for multi-group on WS2022+, got: %v", err)
		}
		if len(limits.GroupAffinities) != 2 {
			t.Fatalf("expected 2 group affinities, got %d: %v", len(limits.GroupAffinities), limits.GroupAffinities)
		}
		want := []jobobject.GroupAffinity{{Mask: 0x1, Group: 0}, {Mask: 0x1, Group: 1}}
		for i, a := range limits.GroupAffinities {
			if a != want[i] {
				t.Fatalf("affinity[%d]: got %v, want %v", i, a, want[i])
			}
		}
	} else {
		if err == nil {
			t.Fatal("expected error for multiple affinity entries on pre-WS2022")
		}
		if !errors.Is(err, hcsoci.ErrCPUAffinityMultipleGroupsNotSupported) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestSpecToLimits_CPUAffinity_NonZeroGroup(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{
			Resources: &specs.WindowsResources{
				CPU: &specs.WindowsCPUResources{
					Affinity: []specs.WindowsCPUGroupAffinity{
						{Mask: 0x1, Group: 1},
					},
				},
			},
		},
	}

	limits, err := specToLimits(context.Background(), "cid", s)
	if osversion.Build() >= osversion.LTSC2022 {
		// Non-zero group is supported on WS2022+.
		if err != nil {
			t.Fatalf("expected success for non-zero group on WS2022+, got: %v", err)
		}
		if len(limits.GroupAffinities) != 1 || limits.GroupAffinities[0].Group != 1 {
			t.Fatalf("unexpected group affinities: got %v", limits.GroupAffinities)
		}
	} else {
		if err == nil {
			t.Fatal("expected error for non-zero affinity group on pre-WS2022")
		}
		if !errors.Is(err, hcsoci.ErrCPUAffinityNonZeroGroupNotSupported) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestSpecToLimits_CPUAffinity_ZeroMaskRejected(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{
			Resources: &specs.WindowsResources{
				CPU: &specs.WindowsCPUResources{
					Affinity: []specs.WindowsCPUGroupAffinity{
						{Mask: 0, Group: 0},
					},
				},
			},
		},
	}

	_, err := specToLimits(context.Background(), "cid", s)
	if err == nil {
		t.Fatal("expected error for zero affinity mask")
	}
	if !errors.Is(err, hcsoci.ErrCPUAffinityMaskZero) {
		t.Fatalf("unexpected error: %v", err)
	}
}
