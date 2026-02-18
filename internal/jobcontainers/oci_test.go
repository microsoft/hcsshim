//go:build windows

package jobcontainers

import (
	"context"
	"strings"
	"testing"

	specs "github.com/opencontainers/runtime-spec/specs-go"
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
	if limits.CPUAffinity != 0x3 {
		t.Fatalf("unexpected cpu affinity: got %d want %d", limits.CPUAffinity, uint64(0x3))
	}
}

func TestSpecToLimits_CPUAffinity_MultiGroupRejected(t *testing.T) {
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

	_, err := specToLimits(context.Background(), "cid", s)
	if err == nil {
		t.Fatal("expected error for multiple affinity entries")
	}
	if !strings.Contains(err.Error(), "multiple processor groups") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSpecToLimits_CPUAffinity_NonZeroGroupRejected(t *testing.T) {
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

	_, err := specToLimits(context.Background(), "cid", s)
	if err == nil {
		t.Fatal("expected error for non-zero affinity group")
	}
	if !strings.Contains(err.Error(), "processor group") {
		t.Fatalf("unexpected error: %v", err)
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
	if !strings.Contains(err.Error(), "mask must be non-zero") {
		t.Fatalf("unexpected error: %v", err)
	}
}
