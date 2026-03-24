//go:build linux
// +build linux

package hcsv2

import (
	"testing"

	v2 "github.com/containerd/cgroups/v3/cgroup2/stats"

	"github.com/Microsoft/hcsshim/internal/guest/cgroup"
)

func TestIsCgroupV2(t *testing.T) {
	// Test the cgroup version detection function returns consistent results
	result1 := cgroup.IsCgroupV2()
	result2 := cgroup.IsCgroupV2()

	t.Logf("Detected cgroup version: v%d", map[bool]int{true: 2, false: 1}[result1])

	if result1 != result2 {
		t.Error("IsCgroupV2() should return consistent results across calls")
	}
}

func TestConvertV2StatsToV1_Memory(t *testing.T) {
	// Test the v2 to v1 metrics conversion for memory stats
	v2Stats := &v2.Metrics{
		Memory: &v2.MemoryStat{
			Usage:        1048576,   // 1MB
			UsageLimit:   268435456, // 256MB
			MaxUsage:     2097152,   // 2MB
			SwapUsage:    512000,    // 500KB
			SwapLimit:    536870912, // 512MB
			SwapMaxUsage: 1024000,   // 1000KB
			File:         131072,    // 128KB cache
			Anon:         917504,    // 896KB RSS
		},
		MemoryEvents: &v2.MemoryEvents{
			OomKill: 5,
		},
	}

	v1Stats := cgroup.ConvertV2StatsToV1(v2Stats)

	// Verify memory stats conversion
	if v1Stats.Memory == nil {
		t.Fatal("Memory stats not converted")
	}

	if v1Stats.Memory.Usage.Usage != 1048576 {
		t.Errorf("Expected usage 1048576, got %d", v1Stats.Memory.Usage.Usage)
	}

	if v1Stats.Memory.Usage.Limit != 268435456 {
		t.Errorf("Expected limit 268435456, got %d", v1Stats.Memory.Usage.Limit)
	}

	if v1Stats.Memory.Usage.Max != 2097152 {
		t.Errorf("Expected max 2097152, got %d", v1Stats.Memory.Usage.Max)
	}

	if v1Stats.Memory.Cache != 131072 {
		t.Errorf("Expected cache 131072, got %d", v1Stats.Memory.Cache)
	}

	if v1Stats.Memory.RSS != 917504 {
		t.Errorf("Expected RSS 917504, got %d", v1Stats.Memory.RSS)
	}

	// Verify swap stats
	if v1Stats.Memory.Swap.Usage != 512000 {
		t.Errorf("Expected swap usage 512000, got %d", v1Stats.Memory.Swap.Usage)
	}

	// Verify OOM control conversion
	if v1Stats.MemoryOomControl == nil {
		t.Fatal("MemoryOomControl not converted")
	}

	if v1Stats.MemoryOomControl.OomKill != 5 {
		t.Errorf("Expected OOM kills 5, got %d", v1Stats.MemoryOomControl.OomKill)
	}
}

func TestConvertV2StatsToV1_CPU(t *testing.T) {
	// Test CPU stats conversion
	v2Stats := &v2.Metrics{
		CPU: &v2.CPUStat{
			UsageUsec:     5000000, // 5 seconds in microseconds
			UserUsec:      2000000, // 2 seconds user
			SystemUsec:    3000000, // 3 seconds system
			NrPeriods:     1000,    // 1000 periods
			NrThrottled:   50,      // 50 throttled periods
			ThrottledUsec: 1500000, // 1.5 seconds throttled
		},
	}

	v1Stats := cgroup.ConvertV2StatsToV1(v2Stats)

	// Verify CPU stats conversion
	if v1Stats.CPU == nil {
		t.Fatal("CPU stats not converted")
	}

	if v1Stats.CPU.Usage == nil {
		t.Fatal("CPU usage not converted")
	}

	// Check microsecond to nanosecond conversion
	expectedTotal := uint64(5000000 * 1000) // 5 seconds in nanoseconds
	if v1Stats.CPU.Usage.Total != expectedTotal {
		t.Errorf("Expected total CPU %d ns, got %d ns", expectedTotal, v1Stats.CPU.Usage.Total)
	}

	expectedUser := uint64(2000000 * 1000) // 2 seconds in nanoseconds
	if v1Stats.CPU.Usage.User != expectedUser {
		t.Errorf("Expected user CPU %d ns, got %d ns", expectedUser, v1Stats.CPU.Usage.User)
	}

	expectedKernel := uint64(3000000 * 1000) // 3 seconds in nanoseconds
	if v1Stats.CPU.Usage.Kernel != expectedKernel {
		t.Errorf("Expected kernel CPU %d ns, got %d ns", expectedKernel, v1Stats.CPU.Usage.Kernel)
	}

	// Verify throttling stats
	if v1Stats.CPU.Throttling == nil {
		t.Fatal("CPU throttling not converted")
	}

	if v1Stats.CPU.Throttling.Periods != 1000 {
		t.Errorf("Expected 1000 periods, got %d", v1Stats.CPU.Throttling.Periods)
	}

	if v1Stats.CPU.Throttling.ThrottledPeriods != 50 {
		t.Errorf("Expected 50 throttled periods, got %d", v1Stats.CPU.Throttling.ThrottledPeriods)
	}

	expectedThrottledTime := uint64(1500000 * 1000) // 1.5 seconds in nanoseconds
	if v1Stats.CPU.Throttling.ThrottledTime != expectedThrottledTime {
		t.Errorf("Expected throttled time %d ns, got %d ns", expectedThrottledTime, v1Stats.CPU.Throttling.ThrottledTime)
	}
}

func TestConvertV2StatsToV1_PIDs(t *testing.T) {
	// Test PIDs stats conversion
	v2Stats := &v2.Metrics{
		Pids: &v2.PidsStat{
			Current: 25,
			Limit:   100,
		},
	}

	v1Stats := cgroup.ConvertV2StatsToV1(v2Stats)

	// Verify PIDs stats conversion
	if v1Stats.Pids == nil {
		t.Fatal("PIDs stats not converted")
	}

	if v1Stats.Pids.Current != 25 {
		t.Errorf("Expected current PIDs 25, got %d", v1Stats.Pids.Current)
	}

	if v1Stats.Pids.Limit != 100 {
		t.Errorf("Expected PID limit 100, got %d", v1Stats.Pids.Limit)
	}
}

func TestConvertV2StatsToV1_IO(t *testing.T) {
	// Test Block I/O stats conversion
	v2Stats := &v2.Metrics{
		Io: &v2.IOStat{
			Usage: []*v2.IOEntry{
				{
					Major:  8,
					Minor:  0,
					Rbytes: 4096,
					Wbytes: 2048,
					Rios:   10,
					Wios:   5,
				},
				{
					Major:  8,
					Minor:  1,
					Rbytes: 8192,
					Wbytes: 4096,
					Rios:   20,
					Wios:   10,
				},
			},
		},
	}

	v1Stats := cgroup.ConvertV2StatsToV1(v2Stats)

	// Verify Block I/O stats conversion
	if v1Stats.Blkio == nil {
		t.Fatal("Block I/O stats not converted")
	}

	// Should have 4 entries per device (read bytes, write bytes, read IOs, write IOs) * 2 devices = 8 entries
	expectedBytesEntries := 4 // 2 devices * 2 operations (read/write)
	if len(v1Stats.Blkio.IoServiceBytesRecursive) != expectedBytesEntries {
		t.Errorf("Expected %d IoServiceBytesRecursive entries, got %d", expectedBytesEntries, len(v1Stats.Blkio.IoServiceBytesRecursive))
	}

	expectedIOsEntries := 4 // 2 devices * 2 operations (read/write)
	if len(v1Stats.Blkio.IoServicedRecursive) != expectedIOsEntries {
		t.Errorf("Expected %d IoServicedRecursive entries, got %d", expectedIOsEntries, len(v1Stats.Blkio.IoServicedRecursive))
	}

	// Verify specific entries exist
	found := false
	for _, entry := range v1Stats.Blkio.IoServiceBytesRecursive {
		if entry.Major == 8 && entry.Minor == 0 && entry.Op == "Read" && entry.Value == 4096 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected read bytes entry for device 8:0 not found")
	}
}

func TestConvertV2StatsToV1_RDMA(t *testing.T) {
	// Test RDMA stats conversion
	v2Stats := &v2.Metrics{
		Rdma: &v2.RdmaStat{
			Current: []*v2.RdmaEntry{
				{
					Device:     "mlx5_0",
					HcaHandles: 10,
					HcaObjects: 5,
				},
			},
			Limit: []*v2.RdmaEntry{
				{
					Device:     "mlx5_0",
					HcaHandles: 100,
					HcaObjects: 50,
				},
			},
		},
	}

	v1Stats := cgroup.ConvertV2StatsToV1(v2Stats)

	// Verify RDMA stats conversion
	if v1Stats.Rdma == nil {
		t.Fatal("RDMA stats not converted")
	}

	if len(v1Stats.Rdma.Current) != 1 {
		t.Errorf("Expected 1 current RDMA entry, got %d", len(v1Stats.Rdma.Current))
	}

	if len(v1Stats.Rdma.Limit) != 1 {
		t.Errorf("Expected 1 limit RDMA entry, got %d", len(v1Stats.Rdma.Limit))
	}

	currentEntry := v1Stats.Rdma.Current[0]
	if currentEntry.Device != "mlx5_0" {
		t.Errorf("Expected device mlx5_0, got %s", currentEntry.Device)
	}

	if currentEntry.HcaHandles != 10 {
		t.Errorf("Expected 10 HCA handles, got %d", currentEntry.HcaHandles)
	}
}

func TestConvertV2StatsToV1_Hugetlb(t *testing.T) {
	// Test Hugetlb stats conversion
	v2Stats := &v2.Metrics{
		Hugetlb: []*v2.HugeTlbStat{
			{
				Current: 2097152, // 2MB
				Max:     4194304, // 4MB
			},
			{
				Current: 1048576, // 1MB
				Max:     2097152, // 2MB
			},
		},
	}

	v1Stats := cgroup.ConvertV2StatsToV1(v2Stats)

	// Verify Hugetlb stats conversion
	if len(v1Stats.Hugetlb) != 2 {
		t.Errorf("Expected 2 hugetlb entries, got %d", len(v1Stats.Hugetlb))
	}

	if v1Stats.Hugetlb[0].Usage != 2097152 {
		t.Errorf("Expected first hugetlb usage 2097152, got %d", v1Stats.Hugetlb[0].Usage)
	}

	if v1Stats.Hugetlb[0].Max != 4194304 {
		t.Errorf("Expected first hugetlb max 4194304, got %d", v1Stats.Hugetlb[0].Max)
	}
}

func TestConvertV2StatsToV1_EmptyMetrics(t *testing.T) {
	// Test conversion with empty v2 metrics
	v2Stats := &v2.Metrics{}

	v1Stats := cgroup.ConvertV2StatsToV1(v2Stats)

	// Should return a valid but empty v1 metrics structure
	if v1Stats == nil {
		t.Fatal("Expected non-nil v1 metrics")
	}

	// All fields should be nil for empty input
	if v1Stats.Memory != nil {
		t.Error("Expected nil memory stats for empty input")
	}

	if v1Stats.CPU != nil {
		t.Error("Expected nil CPU stats for empty input")
	}

	if v1Stats.Pids != nil {
		t.Error("Expected nil PIDs stats for empty input")
	}
}

// Benchmark the v2 to v1 conversion performance
func BenchmarkConvertV2StatsToV1(b *testing.B) {
	// Create a comprehensive v2 stats object for benchmarking
	v2Stats := &v2.Metrics{
		Memory: &v2.MemoryStat{
			Usage:      1048576,
			UsageLimit: 268435456,
			MaxUsage:   2097152,
			File:       131072,
			Anon:       917504,
		},
		CPU: &v2.CPUStat{
			UsageUsec:     5000000,
			UserUsec:      2000000,
			SystemUsec:    3000000,
			NrPeriods:     1000,
			NrThrottled:   50,
			ThrottledUsec: 1500000,
		},
		Pids: &v2.PidsStat{
			Current: 25,
			Limit:   100,
		},
		Io: &v2.IOStat{
			Usage: []*v2.IOEntry{
				{Major: 8, Minor: 0, Rbytes: 4096, Wbytes: 2048, Rios: 10, Wios: 5},
				{Major: 8, Minor: 1, Rbytes: 8192, Wbytes: 4096, Rios: 20, Wios: 10},
			},
		},
		MemoryEvents: &v2.MemoryEvents{
			Oom: 5,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cgroup.ConvertV2StatsToV1(v2Stats)
	}
}

// Additional Edge Cases for Stats Conversion
func TestConvertV2StatsToV1_PartialData(t *testing.T) {
	// Test with only Memory data
	v2Stats := &v2.Metrics{
		Memory: &v2.MemoryStat{
			Usage:     1024 * 1024, // 1MB
			SwapUsage: 512 * 1024,  // 512KB
		},
		// CPU and Pids intentionally nil
	}

	v1Stats := cgroup.ConvertV2StatsToV1(v2Stats)
	if v1Stats == nil {
		t.Fatal("cgroup.ConvertV2StatsToV1 should not return nil")
	}
	if v1Stats.Memory == nil {
		t.Fatal("Memory stats should be converted")
	}
	if v1Stats.Memory.Usage == nil {
		t.Fatal("Memory usage should be converted")
	}
	if v1Stats.Memory.Usage.Usage != v2Stats.Memory.Usage {
		t.Errorf("Expected memory usage %d, got %d", v2Stats.Memory.Usage, v1Stats.Memory.Usage.Usage)
	}
	if v1Stats.CPU != nil {
		t.Error("CPU stats should be nil when v2 CPU is nil")
	}
	if v1Stats.Pids != nil {
		t.Error("Pids stats should be nil when v2 Pids is nil")
	}
}

func TestConvertV2StatsToV1_ZeroValues(t *testing.T) {
	// Test with zero values
	v2Stats := &v2.Metrics{
		Memory: &v2.MemoryStat{
			Usage:     0,
			SwapUsage: 0,
		},
		CPU: &v2.CPUStat{
			UsageUsec: 0,
			UserUsec:  0,
		},
		Pids: &v2.PidsStat{
			Current: 0,
			Limit:   0,
		},
	}

	v1Stats := cgroup.ConvertV2StatsToV1(v2Stats)
	if v1Stats == nil {
		t.Fatal("cgroup.ConvertV2StatsToV1 should not return nil")
	}

	// Verify zero values are preserved
	if v1Stats.Memory.Usage.Usage != 0 {
		t.Error("Zero memory usage should be preserved")
	}
	if v1Stats.CPU.Usage.Total != 0 {
		t.Error("Zero CPU usage should be preserved")
	}
	if v1Stats.Pids.Current != 0 {
		t.Error("Zero PIDs current should be preserved")
	}
}

func TestConvertV2StatsToV1_MaxValues(t *testing.T) {
	// Test with maximum possible values
	maxUint64 := ^uint64(0)
	v2Stats := &v2.Metrics{
		Memory: &v2.MemoryStat{
			Usage:        maxUint64,
			UsageLimit:   maxUint64,
			SwapUsage:    maxUint64,
			MaxUsage:     maxUint64,
			Anon:         maxUint64,
			File:         maxUint64,
			FileMapped:   maxUint64,
			FileDirty:    maxUint64,
			Pgfault:      maxUint64,
			Pgmajfault:   maxUint64,
			InactiveAnon: maxUint64,
			ActiveAnon:   maxUint64,
			InactiveFile: maxUint64,
			ActiveFile:   maxUint64,
			Unevictable:  maxUint64,
		},
		CPU: &v2.CPUStat{
			UsageUsec:     maxUint64,
			UserUsec:      maxUint64,
			SystemUsec:    maxUint64,
			NrPeriods:     maxUint64,
			NrThrottled:   maxUint64,
			ThrottledUsec: maxUint64,
		},
		Pids: &v2.PidsStat{
			Current: maxUint64,
			Limit:   maxUint64,
		},
	}

	v1Stats := cgroup.ConvertV2StatsToV1(v2Stats)
	if v1Stats == nil {
		t.Fatal("cgroup.ConvertV2StatsToV1 should handle max values")
	}

	// Verify max values are handled properly
	if v1Stats.Memory.Usage.Usage != maxUint64 {
		t.Error("Max memory usage should be preserved")
	}
	if v1Stats.Memory.RSS != maxUint64 {
		t.Error("Max RSS should be preserved")
	}
	// Note: CPU conversion multiplies by 1000 for usec to nsec, which may overflow
	// This is expected behavior that should be documented
	if v1Stats.Pids.Current != maxUint64 {
		t.Error("Max PIDs current should be preserved")
	}
}

func TestConvertV2StatsToV1_NilInput(t *testing.T) {
	v1Stats := cgroup.ConvertV2StatsToV1(nil)
	if v1Stats == nil {
		t.Fatal("ConvertV2StatsToV1(nil) should return non-nil empty metrics")
	}
	if v1Stats.Memory != nil || v1Stats.CPU != nil || v1Stats.Pids != nil {
		t.Error("ConvertV2StatsToV1(nil) should return empty metrics with all nil fields")
	}
}

func TestHost_InitializeVirtualPodSupport_ErrorCases(t *testing.T) {
	host := &Host{}

	// Test with nil input
	err := host.InitializeVirtualPodSupport(nil)
	if err == nil {
		t.Error("Expected error for nil input")
	}
	if err != nil && err.Error() != "no valid cgroup manager provided for virtual pod support" {
		t.Errorf("Unexpected error message: %s", err.Error())
	}
}
