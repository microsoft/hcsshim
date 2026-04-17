//go:build windows

package hcsoci

import (
	"strings"
	"testing"

	specs "github.com/opencontainers/runtime-spec/specs-go"
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

	affinity, err := ConvertCPUAffinity(s)
	if err != nil {
		t.Fatalf("ConvertCPUAffinity failed: %v", err)
	}
	if affinity != 0x3 {
		t.Fatalf("unexpected cpu affinity: got %d want %d", affinity, uint64(0x3))
	}
}

func TestConvertCPUAffinity_MultiGroupRejected(t *testing.T) {
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

	_, err := ConvertCPUAffinity(s)
	if err == nil {
		t.Fatal("expected error for multiple affinity entries")
	}
	if !strings.Contains(err.Error(), "multiple processor groups") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConvertCPUAffinity_NonZeroGroupRejected(t *testing.T) {
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

	_, err := ConvertCPUAffinity(s)
	if err == nil {
		t.Fatal("expected error for non-zero affinity group")
	}
	if !strings.Contains(err.Error(), "processor group") {
		t.Fatalf("unexpected error: %v", err)
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
	if !strings.Contains(err.Error(), "mask must be non-zero") {
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
			affinity, err := ConvertCPUAffinity(tc.spec)
			if err != nil {
				t.Fatalf("ConvertCPUAffinity failed: %v", err)
			}
			if affinity != 0 {
				t.Fatalf("expected zero affinity, got %d", affinity)
			}
		})
	}
}
