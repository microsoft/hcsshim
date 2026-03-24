//go:build linux
// +build linux

package cgroup

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	cgroups1 "github.com/containerd/cgroups/v3/cgroup1"
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

func TestMemoryEventMonitoring_V2(t *testing.T) {
	if !IsCgroupV2() {
		t.Skip("Skipping cgroup v2 test on v1 system")
	}

	// Create a temporary cgroup path for testing
	testPath := filepath.Join("/sys/fs/cgroup", "test-memory-event-"+strconv.FormatInt(time.Now().UnixNano(), 10))

	// Clean up after test
	defer func() {
		if _, err := os.Stat(testPath); err == nil {
			os.RemoveAll(testPath)
		}
	}()

	v2mgr := &V2Mgr{path: testPath, done: make(chan struct{})}

	// Test memory event registration with empty event struct
	var event cgroups1.MemoryEvent
	_, err := v2mgr.RegisterMemoryEvent(event)

	// Should not panic even if path doesn't exist
	if err != nil {
		t.Logf("Expected error for non-existent cgroup: %v", err)
	}
}

func TestOOMEventFD_V2_Integration(t *testing.T) {
	if !IsCgroupV2() {
		t.Skip("Skipping cgroup v2 test on v1 system")
	}

	v2mgr := &V2Mgr{path: "/sys/fs/cgroup/test-oom", done: make(chan struct{})}
	defer close(v2mgr.done)

	// Test OOM event FD creation
	fd, err := v2mgr.OOMEventFD()
	if err != nil {
		t.Logf("Expected error for test cgroup: %v", err)
		return
	}

	if fd == 0 {
		t.Error("OOMEventFD should return valid file descriptor")
	}
}

func TestParseMemoryEvents(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected map[string]uint64
	}{
		{
			name:  "Standard memory.events format",
			input: "low 0\nhigh 5\nmax 0\noom 0\noom_kill 3\noom_group_kill 0\n",
			expected: map[string]uint64{
				"low": 0, "high": 5, "max": 0, "oom": 0, "oom_kill": 3, "oom_group_kill": 0,
			},
		},
		{
			name:     "Empty string",
			input:    "",
			expected: map[string]uint64{},
		},
		{
			name:     "Single entry",
			input:    "oom_kill 7",
			expected: map[string]uint64{"oom_kill": 7},
		},
		{
			name:     "Invalid number skipped",
			input:    "oom_kill abc\noom 2",
			expected: map[string]uint64{"oom": 2},
		},
		{
			name:     "Extra fields skipped",
			input:    "oom_kill 3 extra\noom 1",
			expected: map[string]uint64{"oom": 1},
		},
		{
			name:     "Key without value skipped",
			input:    "oom_kill\noom 4",
			expected: map[string]uint64{"oom": 4},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ParseMemoryEvents(tc.input)
			if len(result) != len(tc.expected) {
				t.Errorf("Expected %d events, got %d: %v", len(tc.expected), len(result), result)
				return
			}
			for key, expectedVal := range tc.expected {
				if gotVal, ok := result[key]; !ok {
					t.Errorf("Missing key %q", key)
				} else if gotVal != expectedVal {
					t.Errorf("Key %q: expected %d, got %d", key, expectedVal, gotVal)
				}
			}
		})
	}
}

func TestMemoryEvent_ErrorHandling(t *testing.T) {
	// Test with invalid path
	invalidMgr := &V2Mgr{path: "/invalid/path", done: make(chan struct{})}

	var event cgroups1.MemoryEvent
	_, err := invalidMgr.RegisterMemoryEvent(event)
	if err == nil {
		t.Error("Expected error for invalid cgroup path")
	}

	// Test with valid manager - should work or give expected error
	validMgr := &V2Mgr{path: "/sys/fs/cgroup", done: make(chan struct{})}
	_, err = validMgr.RegisterMemoryEvent(event)
	if err != nil {
		t.Logf("Expected error for test environment: %v", err)
	}
}

func TestConvertToV2Resources(t *testing.T) {
	// nil input
	result := ConvertToV2Resources(nil)
	if result == nil {
		t.Fatal("ConvertToV2Resources(nil) should return non-nil")
	}

	// Empty resources
	result = ConvertToV2Resources(&oci.LinuxResources{})
	if result == nil {
		t.Fatal("ConvertToV2Resources(empty) should return non-nil")
	}

	// Memory limit
	limit := int64(256 * 1024 * 1024)
	result = ConvertToV2Resources(&oci.LinuxResources{
		Memory: &oci.LinuxMemory{Limit: &limit},
	})
	if result.Memory == nil || result.Memory.Max == nil || *result.Memory.Max != limit {
		t.Error("Memory limit not converted to v2 Max")
	}

	// Memory reservation → Low
	reservation := int64(128 * 1024 * 1024)
	result = ConvertToV2Resources(&oci.LinuxResources{
		Memory: &oci.LinuxMemory{Reservation: &reservation},
	})
	if result.Memory == nil || result.Memory.Low == nil || *result.Memory.Low != reservation {
		t.Error("Memory reservation not converted to v2 Low")
	}

	// Memory swap
	swapLimit := int64(512 * 1024 * 1024)
	result = ConvertToV2Resources(&oci.LinuxResources{
		Memory: &oci.LinuxMemory{Swap: &swapLimit},
	})
	if result.Memory == nil || result.Memory.Swap == nil || *result.Memory.Swap != swapLimit {
		t.Error("Memory swap not converted")
	}

	// Pids
	result = ConvertToV2Resources(&oci.LinuxResources{
		Pids: &oci.LinuxPids{Limit: 500},
	})
	if result.Pids == nil || result.Pids.Max != 500 {
		t.Error("Pids limit not converted")
	}

	// Pids with zero limit should not create v2 Pids
	result = ConvertToV2Resources(&oci.LinuxResources{
		Pids: &oci.LinuxPids{Limit: 0},
	})
	if result.Pids != nil {
		t.Error("Pids with zero limit should not create v2 Pids")
	}

	// CPU with quota and period
	quota := int64(50000)
	period := uint64(100000)
	result = ConvertToV2Resources(&oci.LinuxResources{
		CPU: &oci.LinuxCPU{Quota: &quota, Period: &period},
	})
	if result.CPU == nil {
		t.Fatal("CPU not converted")
	}

	// BlockIO with weight
	weight := uint16(500)
	result = ConvertToV2Resources(&oci.LinuxResources{
		BlockIO: &oci.LinuxBlockIO{Weight: &weight},
	})
	if result.IO == nil || result.IO.BFQ.Weight != 500 {
		t.Error("BlockIO weight not converted")
	}
}

func TestConvertToV2Resources_CPUSharesConversion(t *testing.T) {
	// Test the v1 cpu.shares → v2 cpu.weight formula:
	// weight = 1 + ((shares - 2) * 9999) / 262142
	testCases := []struct {
		name           string
		shares         uint64
		expectedWeight uint64
		expectNil      bool // weight should be unset
	}{
		{name: "minimum shares=2", shares: 2, expectedWeight: 1},
		{name: "default shares=1024", shares: 1024, expectedWeight: 39},
		{name: "maximum shares=262144", shares: 262144, expectedWeight: 10000},
		{name: "below minimum shares=1 clamped to 2", shares: 1, expectedWeight: 1},
		{name: "shares=0 means unset", shares: 0, expectNil: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			shares := tc.shares
			result := ConvertToV2Resources(&oci.LinuxResources{
				CPU: &oci.LinuxCPU{Shares: &shares},
			})
			if tc.expectNil {
				if result.CPU != nil && result.CPU.Weight != nil {
					t.Errorf("Expected nil weight for shares=%d, got %d", tc.shares, *result.CPU.Weight)
				}
				return
			}
			if result.CPU == nil || result.CPU.Weight == nil {
				t.Fatalf("Expected non-nil weight for shares=%d", tc.shares)
			}
			if *result.CPU.Weight != tc.expectedWeight {
				t.Errorf("shares=%d: expected weight %d, got %d", tc.shares, tc.expectedWeight, *result.CPU.Weight)
			}
		})
	}
}
