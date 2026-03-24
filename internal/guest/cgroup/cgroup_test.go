//go:build linux
// +build linux

package cgroup

import (
	"testing"

	cgroups2stats "github.com/containerd/cgroups/v3/cgroup2/stats"
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

func TestManagerInterface_Compatibility(t *testing.T) {
	// Compile-time check: these assignments will fail to compile if the types
	// do not satisfy Manager.
	var _ Manager = &V1Mgr{}
	var _ Manager = &V2Mgr{}
}

func TestConvertToV2Resources_Basic(t *testing.T) {
	limit := int64(1024 * 1024 * 1024) // 1GB

	ociResources := &oci.LinuxResources{
		Memory: &oci.LinuxMemory{
			Limit: &limit,
		},
	}

	v2Resources := ConvertToV2Resources(ociResources)

	if v2Resources == nil {
		t.Fatal("v2Resources should not be nil")
	}
	if v2Resources.Memory == nil {
		t.Fatal("v2Resources.Memory should not be nil")
	}
	if v2Resources.Memory.Max == nil || *v2Resources.Memory.Max != limit {
		t.Errorf("Expected memory max %d, got %v", limit, v2Resources.Memory.Max)
	}
}

func TestIsCgroupV2_Detection(t *testing.T) {
	// Test the cgroup version detection
	result1 := IsCgroupV2()
	result2 := IsCgroupV2()

	// Should be consistent
	if result1 != result2 {
		t.Error("IsCgroupV2() should return consistent results")
	}

	t.Logf("Detected cgroup version: v%d", map[bool]int{false: 1, true: 2}[result1])
}

func TestCgroupManager_InvalidPath(t *testing.T) {
	// Use a path that cannot be created due to read-only filesystem constraints
	v2mgr := &V2Mgr{path: "/proc/nonexistent/path"}
	err := v2mgr.Create(1234)
	if err == nil {
		t.Error("Expected error for invalid cgroup path, got nil")
	} else {
		t.Logf("Expected error for invalid path: %v", err)
	}
}

func TestCgroupManager_PermissionDenied(t *testing.T) {
	// Test with root path that should require permissions
	v2mgr := &V2Mgr{path: "/sys/fs/cgroup/test-permission-denied"}
	err := v2mgr.Create(1234)
	// Error is expected, just ensure it doesn't panic
	if err != nil {
		t.Logf("Expected permission error: %v", err)
	}
}

func TestConvertV2StatsToV1_InvalidInput(t *testing.T) {
	// Test with nil input
	result := ConvertV2StatsToV1(nil)
	if result == nil {
		t.Error("ConvertV2StatsToV1 should handle nil input gracefully")
	}

	// Test with empty metrics
	emptyMetrics := &cgroups2stats.Metrics{}
	result = ConvertV2StatsToV1(emptyMetrics)
	if result == nil {
		t.Error("ConvertV2StatsToV1 should handle empty metrics")
	}
}

func TestConvertV2StatsToV1_OomKillMapping(t *testing.T) {
	// Verify that OomKill maps from v2 OomKill (actual kills), not Oom (reclaim attempts).
	v2Stats := &cgroups2stats.Metrics{
		MemoryEvents: &cgroups2stats.MemoryEvents{
			Oom:     100, // OOM reclaim invocations — should NOT be used
			OomKill: 3,   // Actual process kills — should map to v1 OomKill
		},
	}
	result := ConvertV2StatsToV1(v2Stats)
	if result.MemoryOomControl == nil {
		t.Fatal("MemoryOomControl should not be nil when MemoryEvents is present")
	}
	if result.MemoryOomControl.OomKill != 3 {
		t.Errorf("OomKill = %d, want 3 (should use MemoryEvents.OomKill, not Oom)", result.MemoryOomControl.OomKill)
	}
}

func TestConvertToV2Resources_ExtremeLimits(t *testing.T) {
	// Test with maximum possible values
	maxLimit := int64(^uint64(0) >> 1) // Max int64
	ociResources := &oci.LinuxResources{
		Memory: &oci.LinuxMemory{
			Limit: &maxLimit,
		},
		Pids: &oci.LinuxPids{
			Limit: maxLimit,
		},
	}

	v2Resources := ConvertToV2Resources(ociResources)
	if v2Resources == nil {
		t.Fatal("v2Resources should not be nil")
	}

	// Test with zero values
	zeroLimit := int64(0)
	ociResourcesZero := &oci.LinuxResources{
		Memory: &oci.LinuxMemory{
			Limit: &zeroLimit,
		},
	}

	v2ResourcesZero := ConvertToV2Resources(ociResourcesZero)
	if v2ResourcesZero == nil || v2ResourcesZero.Memory == nil {
		t.Error("Should handle zero limit values")
	}
}
