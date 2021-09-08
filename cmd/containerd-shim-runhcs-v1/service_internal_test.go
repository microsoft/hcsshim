package main

import (
	"reflect"
	"testing"

	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	v1 "github.com/containerd/cgroups/stats/v1"
	"github.com/pkg/errors"
)

func verifyExpectedError(t *testing.T, resp interface{}, actual, expected error) {
	if actual == nil || errors.Cause(actual) != expected {
		t.Fatalf("expected error: %v, got: %v", expected, actual)
	}

	isnil := false
	ty := reflect.TypeOf(resp)
	if ty == nil {
		isnil = true
	} else {
		isnil = reflect.ValueOf(resp).IsNil()
	}
	if !isnil {
		t.Fatalf("expect nil response for error return, got: %v", resp)
	}
}

func verifyExpectedStats(t *testing.T, isWCOW, ownsHost bool, s *stats.Statistics) {
	if isWCOW {
		verifyExpectedWindowsContainerStatistics(t, s.GetWindows())
	} else {
		verifyExpectedCgroupMetrics(t, s.GetLinux())
	}
	if ownsHost {
		verifyExpectedVirtualMachineStatistics(t, s.VM)
	}
}

func verifyExpectedWindowsContainerStatistics(t *testing.T, w *stats.WindowsContainerStatistics) {
	if w == nil {
		t.Fatal("expected non-nil WindowsContainerStatistics")
	}
	if w.UptimeNS != 100 {
		t.Fatalf("expected WindowsContainerStatistics.UptimeNS == 100, got: %d", w.UptimeNS)
	}
	if w.Processor == nil {
		t.Fatal("expected non-nil WindowsContainerStatistics.Processor")
	}
	if w.Processor.TotalRuntimeNS != 100 {
		t.Fatalf("expected WindowsContainerStatistics.Processor.TotalRuntimeNS == 100, got: %d", w.Processor.TotalRuntimeNS)
	}
	if w.Processor.RuntimeUserNS != 100 {
		t.Fatalf("expected WindowsContainerStatistics.Processor.RuntimeUserNS == 100, got: %d", w.Processor.RuntimeUserNS)
	}
	if w.Processor.RuntimeKernelNS != 100 {
		t.Fatalf("expected WindowsContainerStatistics.Processor.RuntimeKernelNS == 100, got: %d", w.Processor.RuntimeKernelNS)
	}
	if w.Memory == nil {
		t.Fatal("expected non-nil WindowsContainerStatistics.Memory")
	}
	if w.Memory.MemoryUsageCommitBytes != 100 {
		t.Fatalf("expected WindowsContainerStatistics.Memory.MemoryUsageCommitBytes == 100, got: %d", w.Memory.MemoryUsageCommitBytes)
	}
	if w.Memory.MemoryUsageCommitPeakBytes != 100 {
		t.Fatalf("expected WindowsContainerStatistics.Memory.MemoryUsageCommitPeakBytes == 100, got: %d", w.Memory.MemoryUsageCommitPeakBytes)
	}
	if w.Memory.MemoryUsagePrivateWorkingSetBytes != 100 {
		t.Fatalf("expected WindowsContainerStatistics.Memory.MemoryUsagePrivateWorkingSetBytes == 100, got: %d", w.Memory.MemoryUsagePrivateWorkingSetBytes)
	}
	if w.Storage == nil {
		t.Fatal("expected non-nil WindowsContainerStatistics.Memory")
	}
	if w.Storage.ReadCountNormalized != 100 {
		t.Fatalf("expected WindowsContainerStatistics.Storage.ReadCountNormalized == 100, got: %d", w.Storage.ReadCountNormalized)
	}
	if w.Storage.ReadSizeBytes != 100 {
		t.Fatalf("expected WindowsContainerStatistics.Storage.ReadSizeBytes == 100, got: %d", w.Storage.ReadSizeBytes)
	}
	if w.Storage.WriteCountNormalized != 100 {
		t.Fatalf("expected WindowsContainerStatistics.Storage.WriteCountNormalized == 100, got: %d", w.Storage.WriteCountNormalized)
	}
	if w.Storage.WriteSizeBytes != 100 {
		t.Fatalf("expected WindowsContainerStatistics.Storage.WriteSizeBytes == 100, got: %d", w.Storage.WriteSizeBytes)
	}
}

func verifyExpectedCgroupMetrics(t *testing.T, v *v1.Metrics) {
	if v == nil {
		t.Fatal("expected non-nil cgroups Metrics")
	}
	if v.CPU == nil {
		t.Fatal("expected non-nil Metrics.CPU")
	}
	if v.CPU.Usage.Total != 100 {
		t.Fatalf("Expected Metrics.CPU.Usage == 100, got: %d", v.CPU.Usage)
	}
	if v.Memory == nil {
		t.Fatal("expected non-nil Metrics.Memory")
	}
	if v.Memory.TotalInactiveFile != 100 {
		t.Fatalf("Expected Metrics.Memory.TotalInactiveFile == 100, got: %d", v.Memory.TotalInactiveFile)
	}
	if v.Memory.Usage == nil {
		t.Fatal("expected non-nil Metrics.Memory.Usage")
	}
	if v.Memory.Usage.Usage != 200 {
		t.Fatalf("Expected Metrics.Memory.Usage.Usage == 200, got: %d", v.Memory.Usage.Usage)
	}
}

func verifyExpectedVirtualMachineStatistics(t *testing.T, v *stats.VirtualMachineStatistics) {
	if v == nil {
		t.Fatal("expected non-nil VirtualMachineStatistics")
	}
	if v.Processor == nil {
		t.Fatal("expected non-nil VirtualMachineStatistics.Processor")
	}
	if v.Processor.TotalRuntimeNS != 100 {
		t.Fatalf("expected VirtualMachineStatistics.Processor.TotalRuntimeNS == 100, got: %d", v.Processor.TotalRuntimeNS)
	}
	if v.Memory == nil {
		t.Fatal("expected non-nil VirtualMachineStatistics.Memory")
	}
	if v.Memory.WorkingSetBytes != 100 {
		t.Fatalf("expected VirtualMachineStatistics.Memory.WorkingSetBytes == 100, got: %d", v.Memory.WorkingSetBytes)
	}
}
