//go:build linux

package gcs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/guest/cgroup"
	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"

	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
)

//
// Functional e2e tests for cgroup v1/v2 compatibility.
//
// These tests run inside the UVM as part of the GCS test binary and exercise
// the full cgroup stats pipeline regardless of which cgroup version the kernel
// provides. They serve as the primary regression gate for the cgroup v2 migration.
//

// cgVersionStr returns "v1" or "v2" for log messages.
func cgVersionStr() string {
	if cgroup.IsCgroupV2() {
		return "v2"
	}
	return "v1"
}

// TestCgroupVersionDetection verifies that the cgroup version detected by the
// GCS matches the actual filesystem layout. If detection is wrong, all cgroup
// operations will fail.
func TestCgroupVersionDetection(t *testing.T) {
	requireFeatures(t, featureStandalone)

	isV2 := cgroup.IsCgroupV2()

	// cgroup v2 unified hierarchy marker
	_, err := os.Stat("/sys/fs/cgroup/cgroup.controllers")
	hasV2Controllers := err == nil

	// cgroup v1 per-subsystem mount marker
	_, err = os.Stat("/sys/fs/cgroup/memory")
	hasV1Memory := err == nil

	t.Logf("IsCgroupV2()=%v  v2Controllers=%v  v1Memory=%v", isV2, hasV2Controllers, hasV1Memory)

	if isV2 && !hasV2Controllers {
		t.Fatal("IsCgroupV2() returned true but /sys/fs/cgroup/cgroup.controllers missing")
	}
	if !isV2 && !hasV1Memory {
		t.Fatal("IsCgroupV2() returned false but /sys/fs/cgroup/memory missing")
	}
}

// TestContainerStats_CgroupMetrics creates a container, runs a workload, then
// queries its cgroup stats via GetProperties(PtStatistics). This exercises:
//
//	container.Start → runc creates cgroup → GetStats → LoadAndStat →
//	  v1: cgroups1.Stat()
//	  v2: cgroups2.Stat() + ConvertV2StatsToV1
//
// This is the primary regression test for the cgroup migration.
func TestContainerStats_CgroupMetrics(t *testing.T) {
	requireFeatures(t, featureStandalone)

	ctx := context.Background()
	host, _ := getTestState(ctx, t)

	id := t.Name()
	c := createStandaloneContainer(ctx, t, host, id,
		oci.WithProcessArgs("/bin/sh", "-c", "head -c 1048576 /dev/urandom > /dev/null; tail -f /dev/null"),
	)
	t.Cleanup(func() { cleanupContainer(ctx, t, host, c) })

	p := startContainer(ctx, t, c, stdio.ConnectionSettings{})
	t.Cleanup(func() {
		killContainer(ctx, t, c)
		waitContainer(ctx, t, c, p, true)
	})

	props, err := host.GetProperties(ctx, id, prot.PropertyQuery{
		PropertyTypes: []prot.PropertyType{prot.PtStatistics},
	})
	if err != nil {
		t.Fatalf("GetProperties(Statistics) failed: %v", err)
	}

	metrics := props.Metrics
	if metrics == nil {
		t.Fatal("expected non-nil metrics")
	}

	v := cgVersionStr()

	// Memory
	if metrics.Memory == nil || metrics.Memory.Usage == nil {
		t.Fatalf("cgroup %s: Memory or Memory.Usage is nil", v)
	}
	if metrics.Memory.Usage.Usage == 0 {
		t.Errorf("cgroup %s: expected non-zero memory usage", v)
	}
	t.Logf("cgroup %s: memory usage=%d limit=%d", v,
		metrics.Memory.Usage.Usage, metrics.Memory.Usage.Limit)

	// CPU
	if metrics.CPU == nil || metrics.CPU.Usage == nil {
		t.Fatalf("cgroup %s: CPU or CPU.Usage is nil", v)
	}
	t.Logf("cgroup %s: cpu total=%d user=%d kernel=%d", v,
		metrics.CPU.Usage.Total, metrics.CPU.Usage.User, metrics.CPU.Usage.Kernel)

	// PIDs
	if metrics.Pids == nil {
		t.Fatalf("cgroup %s: Pids is nil", v)
	}
	if metrics.Pids.Current == 0 {
		t.Errorf("cgroup %s: expected at least 1 pid", v)
	}
	t.Logf("cgroup %s: pids current=%d limit=%d", v,
		metrics.Pids.Current, metrics.Pids.Limit)
}

// TestContainerStats_MemoryLimit creates a container with a specific memory
// limit and verifies the limit is correctly reported in stats. This validates
// that OCI resource specs translate correctly to both cgroup v1
// memory.limit_in_bytes and cgroup v2 memory.max.
func TestContainerStats_MemoryLimit(t *testing.T) {
	requireFeatures(t, featureStandalone)

	ctx := context.Background()
	host, _ := getTestState(ctx, t)

	id := t.Name()
	memoryLimit := int64(128 * 1024 * 1024) // 128 MiB

	c := createStandaloneContainer(ctx, t, host, id,
		oci.WithProcessArgs("/bin/sh", "-c", "tail -f /dev/null"),
		oci.WithMemoryLimit(uint64(memoryLimit)),
	)
	t.Cleanup(func() { cleanupContainer(ctx, t, host, c) })

	p := startContainer(ctx, t, c, stdio.ConnectionSettings{})
	t.Cleanup(func() {
		killContainer(ctx, t, c)
		waitContainer(ctx, t, c, p, true)
	})

	props, err := host.GetProperties(ctx, id, prot.PropertyQuery{
		PropertyTypes: []prot.PropertyType{prot.PtStatistics},
	})
	if err != nil {
		t.Fatalf("GetProperties(Statistics) failed: %v", err)
	}

	metrics := props.Metrics
	if metrics == nil || metrics.Memory == nil || metrics.Memory.Usage == nil {
		t.Fatal("expected non-nil memory metrics")
	}

	v := cgVersionStr()
	reportedLimit := int64(metrics.Memory.Usage.Limit)
	t.Logf("cgroup %s: requested limit=%d  reported limit=%d", v, memoryLimit, reportedLimit)

	if reportedLimit != memoryLimit {
		t.Errorf("cgroup %s: memory limit mismatch: want %d, got %d", v, memoryLimit, reportedLimit)
	}
	if metrics.Memory.Usage.Usage > uint64(memoryLimit) {
		t.Errorf("cgroup %s: usage (%d) exceeds limit (%d)", v,
			metrics.Memory.Usage.Usage, memoryLimit)
	}
}

// TestContainerExec_CgroupIsolation verifies that exec'd processes inherit
// the container's cgroup. On v1 /proc/self/cgroup shows "N:controller:/path",
// on v2 it shows "0::/path". Either way the path must not be "/".
func TestContainerExec_CgroupIsolation(t *testing.T) {
	requireFeatures(t, featureStandalone)

	ctx := namespaces.WithNamespace(context.Background(), testoci.DefaultNamespace)
	host, _ := getTestState(ctx, t)

	id := strings.ReplaceAll(t.Name(), "/", "")
	c := createStandaloneContainer(ctx, t, host, id,
		oci.WithProcessArgs("/bin/sh", "-c", "sleep 3600"),
	)
	t.Cleanup(func() { cleanupContainer(ctx, t, host, c) })

	ip := startContainer(ctx, t, c, stdio.ConnectionSettings{})
	t.Cleanup(func() {
		killContainer(ctx, t, c)
		waitContainer(ctx, t, c, ip, true)
	})

	// Exec "cat /proc/self/cgroup" and capture stdout
	ps := testoci.CreateLinuxSpec(ctx, t, id,
		oci.WithDefaultPathEnv,
		oci.WithProcessArgs("/bin/sh", "-c", "cat /proc/self/cgroup"),
	).Process

	con := newConnectionSettings(false, true, true)
	f := createStdIO(ctx, t, con)

	var outStr string
	g := &errgroup.Group{}
	g.Go(func() error {
		outStr = f.ReadAllOut(ctx, t)
		return nil
	})

	time.Sleep(10 * time.Millisecond)
	ep := execProcess(ctx, t, c, ps, con)

	// Wait for the exec'd process to finish
	exch, dch := ep.Wait()
	defer close(dch)
	e := <-exch
	dch <- true

	if e != 0 {
		t.Fatalf("exec process exited with code %d", e)
	}

	_ = g.Wait()
	t.Logf("/proc/self/cgroup output:\n%s", outStr)

	if outStr == "" {
		t.Fatal("expected non-empty /proc/self/cgroup output")
	}

	// Parse cgroup lines and verify at least one has a non-root path
	foundContainerCgroup := false
	for _, line := range strings.Split(strings.TrimSpace(outStr), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) == 3 {
			cgPath := parts[2]
			if cgPath != "/" && cgPath != "" {
				foundContainerCgroup = true
				t.Logf("container cgroup path: %s", cgPath)
				break
			}
		}
	}

	if !foundContainerCgroup {
		t.Error("exec'd process is in root cgroup, not container cgroup")
	}
}

// TestMultipleContainers_IndependentStats creates two containers with different
// memory limits and verifies their stats are independent, confirming that the
// cgroup hierarchy works correctly on both v1 and v2.
func TestMultipleContainers_IndependentStats(t *testing.T) {
	requireFeatures(t, featureStandalone)

	ctx := context.Background()
	host, _ := getTestState(ctx, t)

	limit1 := int64(64 * 1024 * 1024)  // 64 MiB
	limit2 := int64(128 * 1024 * 1024) // 128 MiB

	id1 := fmt.Sprintf("%s-1", strings.ReplaceAll(t.Name(), "/", ""))
	c1 := createStandaloneContainer(ctx, t, host, id1,
		oci.WithProcessArgs("/bin/sh", "-c", "sleep 3600"),
		oci.WithMemoryLimit(uint64(limit1)),
	)
	t.Cleanup(func() { cleanupContainer(ctx, t, host, c1) })
	p1 := startContainer(ctx, t, c1, stdio.ConnectionSettings{})
	t.Cleanup(func() {
		killContainer(ctx, t, c1)
		waitContainer(ctx, t, c1, p1, true)
	})

	id2 := fmt.Sprintf("%s-2", strings.ReplaceAll(t.Name(), "/", ""))
	c2 := createStandaloneContainer(ctx, t, host, id2,
		oci.WithProcessArgs("/bin/sh", "-c", "sleep 3600"),
		oci.WithMemoryLimit(uint64(limit2)),
	)
	t.Cleanup(func() { cleanupContainer(ctx, t, host, c2) })
	p2 := startContainer(ctx, t, c2, stdio.ConnectionSettings{})
	t.Cleanup(func() {
		killContainer(ctx, t, c2)
		waitContainer(ctx, t, c2, p2, true)
	})

	props1, err := host.GetProperties(ctx, id1, prot.PropertyQuery{
		PropertyTypes: []prot.PropertyType{prot.PtStatistics},
	})
	if err != nil {
		t.Fatalf("GetProperties container 1: %v", err)
	}
	props2, err := host.GetProperties(ctx, id2, prot.PropertyQuery{
		PropertyTypes: []prot.PropertyType{prot.PtStatistics},
	})
	if err != nil {
		t.Fatalf("GetProperties container 2: %v", err)
	}

	m1, m2 := props1.Metrics, props2.Metrics
	if m1 == nil || m1.Memory == nil || m1.Memory.Usage == nil {
		t.Fatal("container 1: missing memory metrics")
	}
	if m2 == nil || m2.Memory == nil || m2.Memory.Usage == nil {
		t.Fatal("container 2: missing memory metrics")
	}

	v := cgVersionStr()
	t.Logf("cgroup %s: c1 limit=%d  c2 limit=%d", v, m1.Memory.Usage.Limit, m2.Memory.Usage.Limit)

	if int64(m1.Memory.Usage.Limit) != limit1 {
		t.Errorf("cgroup %s: container 1 limit=%d, want %d", v, m1.Memory.Usage.Limit, limit1)
	}
	if int64(m2.Memory.Usage.Limit) != limit2 {
		t.Errorf("cgroup %s: container 2 limit=%d, want %d", v, m2.Memory.Usage.Limit, limit2)
	}
}

// TestContainerStats_CPUThrottling creates a container with a CPU CFS quota
// and runs a busy loop, then verifies the throttling stats are reported. This
// validates that OCI CPU quota/period translate correctly to both cgroup v1
// cpu.cfs_quota_us/cpu.cfs_period_us and cgroup v2 cpu.max.
func TestContainerStats_CPUThrottling(t *testing.T) {
	requireFeatures(t, featureStandalone)

	ctx := context.Background()
	host, _ := getTestState(ctx, t)

	id := strings.ReplaceAll(t.Name(), "/", "")

	// 10ms quota per 100ms period = 10% of one CPU.
	// A tight burn loop will be throttled heavily.
	var (
		cpuQuota  int64  = 10000  // 10ms in µs
		cpuPeriod uint64 = 100000 // 100ms in µs
	)

	c := createStandaloneContainer(ctx, t, host, id,
		// Tight burn loop in background; keep container alive.
		oci.WithProcessArgs("/bin/sh", "-c", "while true; do :; done"),
		oci.WithCPUCFS(cpuQuota, cpuPeriod),
	)
	t.Cleanup(func() { cleanupContainer(ctx, t, host, c) })

	p := startContainer(ctx, t, c, stdio.ConnectionSettings{})
	t.Cleanup(func() {
		killContainer(ctx, t, c)
		waitContainer(ctx, t, c, p, true)
	})

	// Let the burn loop run enough scheduling periods to accumulate throttling.
	time.Sleep(2 * time.Second)

	props, err := host.GetProperties(ctx, id, prot.PropertyQuery{
		PropertyTypes: []prot.PropertyType{prot.PtStatistics},
	})
	if err != nil {
		t.Fatalf("GetProperties(Statistics) failed: %v", err)
	}

	metrics := props.Metrics
	if metrics == nil {
		t.Fatal("expected non-nil metrics")
	}

	v := cgVersionStr()

	if metrics.CPU == nil || metrics.CPU.Usage == nil {
		t.Fatalf("cgroup %s: CPU or CPU.Usage is nil", v)
	}
	if metrics.CPU.Usage.Total == 0 {
		t.Errorf("cgroup %s: expected non-zero cpu total", v)
	}

	if metrics.CPU.Throttling == nil {
		t.Fatalf("cgroup %s: Throttling is nil", v)
	}

	t.Logf("cgroup %s: cpu total=%d  periods=%d  throttled_periods=%d  throttled_time=%d",
		v,
		metrics.CPU.Usage.Total,
		metrics.CPU.Throttling.Periods,
		metrics.CPU.Throttling.ThrottledPeriods,
		metrics.CPU.Throttling.ThrottledTime,
	)

	if metrics.CPU.Throttling.Periods == 0 {
		t.Errorf("cgroup %s: expected Periods > 0 with CFS quota set", v)
	}
	if metrics.CPU.Throttling.ThrottledPeriods == 0 {
		t.Errorf("cgroup %s: expected ThrottledPeriods > 0 for a burn loop at 10%% quota", v)
	}
	if metrics.CPU.Throttling.ThrottledTime == 0 {
		t.Errorf("cgroup %s: expected ThrottledTime > 0", v)
	}
}

// TestOOMEventFD_ContainerKill creates a container with a small memory limit,
// triggers an OOM kill via an exec'd subprocess, then verifies:
//  1. The OOM kill is reflected in cgroup stats (MemoryOomControl.OomKill > 0).
//  2. The OOMEventFD fires (eventfd becomes readable) on the container's cgroup.
//
// This exercises the full OOM detection pipeline including the v2 eventfd
// emulation that polls memory.events for oom_kill increments.
func TestOOMEventFD_ContainerKill(t *testing.T) {
	requireFeatures(t, featureStandalone)

	ctx := namespaces.WithNamespace(context.Background(), testoci.DefaultNamespace)
	host, _ := getTestState(ctx, t)

	id := strings.ReplaceAll(t.Name(), "/", "")
	memoryLimit := uint64(64 * 1024 * 1024) // 64 MiB

	c := createStandaloneContainer(ctx, t, host, id,
		oci.WithProcessArgs("/bin/sh", "-c", "sleep 3600"),
		oci.WithMemoryLimit(memoryLimit),
	)
	t.Cleanup(func() { cleanupContainer(ctx, t, host, c) })

	ip := startContainer(ctx, t, c, stdio.ConnectionSettings{})
	t.Cleanup(func() {
		killContainer(ctx, t, c)
		waitContainer(ctx, t, c, ip, true)
	})

	// --- Set up OOMEventFD on the container's cgroup ---
	//
	// Standalone containers get cgroup path "/containers/<id>".
	cgroupPath := "/containers/" + id

	// LoadManager works for both v1 and v2 — the cgroup already exists (created by runc).
	mgr, err := cgroup.LoadManager(cgroupPath)
	if err != nil {
		t.Fatalf("LoadManager(%s): %v", cgroupPath, err)
	}
	oomFD, err := mgr.OOMEventFD()
	if err != nil {
		t.Fatalf("OOMEventFD: %v", err)
	}
	oomFile := os.NewFile(oomFD, "oom-eventfd")
	t.Cleanup(func() { oomFile.Close() })

	// --- Trigger OOM via exec ---
	//
	// Write 128 MiB to /dev/shm (tmpfs, backed by memory) inside a 64 MiB
	// container. The writing process will be OOM-killed by the kernel.
	ps := testoci.CreateLinuxSpec(ctx, t, id,
		oci.WithDefaultPathEnv,
		oci.WithProcessArgs("/bin/sh", "-c",
			"dd if=/dev/urandom of=/dev/shm/fill bs=1M count=128 2>/dev/null; exit 0"),
	).Process

	con := newConnectionSettings(false, false, false)
	ep := execProcess(ctx, t, c, ps, con)

	exch, dch := ep.Wait()
	defer close(dch)
	exitCode := <-exch
	dch <- true

	t.Logf("exec process exited with code %d (expected non-zero due to OOM kill)", exitCode)

	// Give the kernel a moment to update cgroup counters.
	time.Sleep(500 * time.Millisecond)

	// --- Verify OOM kill in stats ---
	props, err := host.GetProperties(ctx, id, prot.PropertyQuery{
		PropertyTypes: []prot.PropertyType{prot.PtStatistics},
	})
	if err != nil {
		t.Fatalf("GetProperties(Statistics) after OOM: %v", err)
	}

	v := cgVersionStr()
	metrics := props.Metrics
	if metrics == nil {
		t.Fatal("expected non-nil metrics after OOM")
	}

	if metrics.MemoryOomControl == nil {
		t.Fatalf("cgroup %s: MemoryOomControl is nil after OOM kill", v)
	}
	t.Logf("cgroup %s: OomKill=%d", v, metrics.MemoryOomControl.OomKill)

	if metrics.MemoryOomControl.OomKill == 0 {
		t.Errorf("cgroup %s: expected OomKill > 0 after triggering OOM", v)
	}

	// --- Verify OOMEventFD fired ---
	//
	// The eventfd should be readable. Use a non-blocking read with a timeout
	// to avoid hanging if the notification was missed.
	buf := make([]byte, 8)
	if err := unix.SetNonblock(int(oomFD), true); err != nil {
		t.Fatalf("SetNonblock on oom eventfd: %v", err)
	}

	// Poll with a generous timeout — v2 emulation polls every 1s.
	deadline := time.Now().Add(5 * time.Second)
	eventFired := false
	for time.Now().Before(deadline) {
		n, err := unix.Read(int(oomFD), buf)
		if n > 0 {
			eventFired = true
			break
		}
		if err == unix.EAGAIN {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err != nil {
			t.Fatalf("read from oom eventfd: %v", err)
		}
	}

	if !eventFired {
		t.Errorf("cgroup %s: OOMEventFD did not fire within 5s after OOM kill", v)
	} else {
		t.Logf("cgroup %s: OOMEventFD fired successfully", v)
	}
}
