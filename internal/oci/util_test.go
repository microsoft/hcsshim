package oci

import (
	"testing"

	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func Test_IsLCOW_WCOW(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{},
	}
	if IsLCOW(s) {
		t.Fatal("should not have returned LCOW spec for WCOW config")
	}
}

func Test_IsLCOW_WCOW_Isolated(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
	}
	if IsLCOW(s) {
		t.Fatal("should not have returned LCOW spec for WCOW isolated config")
	}
}

func Test_IsLCOW_Success(t *testing.T) {
	s := &specs.Spec{
		Linux: &specs.Linux{},
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
	}
	if !IsLCOW(s) {
		t.Fatal("should have returned LCOW spec")
	}
}

func Test_IsLCOW_NoWindows_Success(t *testing.T) {
	s := &specs.Spec{
		Linux: &specs.Linux{},
	}
	if !IsLCOW(s) {
		t.Fatal("should have returned LCOW spec")
	}
}

func Test_IsLCOW_Neither(t *testing.T) {
	s := &specs.Spec{}

	if IsLCOW(s) {
		t.Fatal("should not have returned LCOW spec for neither config")
	}
}

func Test_IsWCOW_Success(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{},
	}
	if !IsWCOW(s) {
		t.Fatal("should have returned WCOW spec for WCOW config")
	}
}

func Test_IsWCOW_Isolated_Success(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
	}
	if !IsWCOW(s) {
		t.Fatal("should have returned WCOW spec for WCOW isolated config")
	}
}

func Test_IsWCOW_LCOW(t *testing.T) {
	s := &specs.Spec{
		Linux: &specs.Linux{},
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
	}
	if IsWCOW(s) {
		t.Fatal("should not have returned WCOW spec for LCOW config")
	}
}

func Test_IsWCOW_LCOW_NoWindows_Success(t *testing.T) {
	s := &specs.Spec{
		Linux: &specs.Linux{},
	}
	if IsWCOW(s) {
		t.Fatal("should not have returned WCOW spec for LCOW config")
	}
}

func Test_IsWCOW_Neither(t *testing.T) {
	s := &specs.Spec{}

	if IsWCOW(s) {
		t.Fatal("should not have returned WCOW spec for neither config")
	}
}

func Test_IsIsolated_WCOW(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{},
	}
	if IsIsolated(s) {
		t.Fatal("should not have returned isolated for WCOW config")
	}
}

func Test_IsIsolated_WCOW_Isolated(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
	}
	if !IsIsolated(s) {
		t.Fatal("should have returned isolated for WCOW isolated config")
	}
}

func Test_IsIsolated_LCOW(t *testing.T) {
	s := &specs.Spec{
		Linux: &specs.Linux{},
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
	}
	if !IsIsolated(s) {
		t.Fatal("should have returned isolated for LCOW config")
	}
}

func Test_IsIsolated_LCOW_NoWindows(t *testing.T) {
	s := &specs.Spec{
		Linux: &specs.Linux{},
	}
	if !IsIsolated(s) {
		t.Fatal("should have returned isolated for LCOW config")
	}
}

func Test_IsIsolated_Neither(t *testing.T) {
	s := &specs.Spec{}

	if IsIsolated(s) {
		t.Fatal("should have not have returned isolated for neither config")
	}
}

// -----------------------------------------------------------------------------
// IsJobContainer tests
// -----------------------------------------------------------------------------

func Test_IsJobContainer(t *testing.T) {
	tests := []struct {
		name     string
		spec     *specs.Spec
		expected bool
	}{
		{
			name: "WCOW process-isolated with HostProcess=true",
			spec: &specs.Spec{
				Windows:     &specs.Windows{},
				Annotations: map[string]string{annotations.HostProcessContainer: "true"},
			},
			expected: true,
		},
		{
			name: "WCOW process-isolated with HostProcess=false",
			spec: &specs.Spec{
				Windows:     &specs.Windows{},
				Annotations: map[string]string{annotations.HostProcessContainer: "false"},
			},
			expected: false,
		},
		{
			name: "WCOW process-isolated with HostProcess missing",
			spec: &specs.Spec{
				Windows: &specs.Windows{},
			},
			expected: false,
		},
		{
			name: "WCOW Hyper-V isolated with HostProcess=true",
			spec: &specs.Spec{
				Windows: &specs.Windows{
					HyperV: &specs.WindowsHyperV{},
				},
				Annotations: map[string]string{annotations.HostProcessContainer: "true"},
			},
			expected: true,
		},
		{
			name: "LCOW without Windows (not a JobContainer)",
			spec: &specs.Spec{
				Linux: &specs.Linux{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			actual := IsJobContainer(tt.spec)
			if actual != tt.expected {
				t.Fatalf("IsJobContainer() = %v, expected %v", actual, tt.expected)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Cross-property tests: IsJobContainer combined with IsIsolated to differentiate
// process-isolated HPC from hypervisor-isolated HPC.
// -----------------------------------------------------------------------------

func Test_JobContainer_IsolationDifferentiation(t *testing.T) {
	// Process-isolated WCOW HostProcess=true
	processJob := &specs.Spec{
		Windows:     &specs.Windows{},
		Annotations: map[string]string{annotations.HostProcessContainer: "true"},
	}
	if !IsJobContainer(processJob) {
		t.Fatal("expected IsJobContainer to be true for process-isolated HostProcess=true")
	}
	if IsIsolated(processJob) {
		t.Fatal("expected IsIsolated to be false for process-isolated HostProcess=true")
	}

	// Hyper-V isolated WCOW HostProcess=true
	hyperVJob := &specs.Spec{
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
		Annotations: map[string]string{annotations.HostProcessContainer: "true"},
	}
	if !IsJobContainer(hyperVJob) {
		t.Fatal("expected IsJobContainer to be true for Hyper-V isolated HostProcess=true")
	}
	if !IsIsolated(hyperVJob) {
		t.Fatal("expected IsIsolated to be true for Hyper-V isolated HostProcess=true")
	}
}
