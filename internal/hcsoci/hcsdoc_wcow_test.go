//go:build windows

package hcsoci

import (
	"errors"
	"testing"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/osversion"
)

func TestConvertCPUAffinity_Group0MaskSet(t *testing.T) {
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

	affinities, err := ConvertCPUAffinity(s)
	if err != nil {
		t.Fatalf("ConvertCPUAffinity failed: %v", err)
	}
	if len(affinities) != 1 || affinities[0].Mask != 0x3 || affinities[0].Group != 0 {
		t.Fatalf("unexpected cpu affinity: got %v", affinities)
	}
}

func TestConvertCPUAffinity_MultiGroup(t *testing.T) {
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

	affinities, err := ConvertCPUAffinity(s)
	if osversion.Build() >= osversion.LTSC2022 {
		// Multi-group is supported on WS2022+.
		if err != nil {
			t.Fatalf("expected success for multi-group on WS2022+, got: %v", err)
		}
		if len(affinities) != 2 {
			t.Fatalf("expected 2 affinity entries, got %d", len(affinities))
		}
	} else {
		if err == nil {
			t.Fatal("expected error for multiple affinity entries on pre-WS2022")
		}
		if !errors.Is(err, ErrCPUAffinityMultipleGroupsNotSupported) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestConvertCPUAffinity_NonZeroGroup(t *testing.T) {
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

	affinities, err := ConvertCPUAffinity(s)
	if osversion.Build() >= osversion.LTSC2022 {
		// Non-zero group is supported on WS2022+.
		if err != nil {
			t.Fatalf("expected success for non-zero group on WS2022+, got: %v", err)
		}
		if len(affinities) != 1 || affinities[0].Group != 1 {
			t.Fatalf("unexpected affinity: got %v", affinities)
		}
	} else {
		if err == nil {
			t.Fatal("expected error for non-zero affinity group on pre-WS2022")
		}
		if !errors.Is(err, ErrCPUAffinityNonZeroGroupNotSupported) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestConvertCPUAffinity_ZeroMaskRejected(t *testing.T) {
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

	_, err := ConvertCPUAffinity(s)
	if err == nil {
		t.Fatal("expected error for zero affinity mask")
	}
	if !errors.Is(err, ErrCPUAffinityMaskZero) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConvertCPUAffinity_NoAffinity(t *testing.T) {
	testCases := []struct {
		name string
		spec *specs.Spec
	}{
		{
			name: "nil spec.Windows",
			spec: &specs.Spec{},
		},
		{
			name: "nil spec.Windows.Resources",
			spec: &specs.Spec{
				Windows: &specs.Windows{},
			},
		},
		{
			name: "nil spec.Windows.Resources.CPU",
			spec: &specs.Spec{
				Windows: &specs.Windows{
					Resources: &specs.WindowsResources{},
				},
			},
		},
		{
			name: "empty affinity slice",
			spec: &specs.Spec{
				Windows: &specs.Windows{
					Resources: &specs.WindowsResources{
						CPU: &specs.WindowsCPUResources{
							Affinity: []specs.WindowsCPUGroupAffinity{},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			affinities, err := ConvertCPUAffinity(tc.spec)
			if err != nil {
				t.Fatalf("ConvertCPUAffinity failed: %v", err)
			}
			if len(affinities) != 0 {
				t.Fatalf("expected empty affinities, got %v", affinities)
			}
		})
	}
}
