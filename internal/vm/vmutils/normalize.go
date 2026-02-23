//go:build windows

package vmutils

import (
	"context"
	"runtime"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/sirupsen/logrus"
)

// DefaultProcessorCountForUVM returns the default processor count to use for a utility VM.
// If the system has only 1 logical processor, it returns 1. Otherwise, it returns 2.
func DefaultProcessorCountForUVM() int32 {
	if runtime.NumCPU() == 1 {
		return 1
	}
	return 2
}

// NormalizeMemorySize aligns the requested memory size to an even number (2MB alignment).
func NormalizeMemorySize(ctx context.Context, uvmID string, requested uint64) uint64 {
	actual := (requested + 1) &^ 1 // align up to an even number
	if requested != actual {
		log.G(ctx).WithFields(logrus.Fields{
			logfields.UVMID: uvmID,
			"requested":     requested,
			"assigned":      actual,
		}).Warn("Changing user requested MemorySizeInMB to align to 2MB")
	}
	return actual
}

// NormalizeProcessorCount ensures that the requested processor count does not exceed the host's logical processor count as reported by HCS.
func NormalizeProcessorCount(ctx context.Context, uvmID string, requested int32, processorTopology *hcsschema.ProcessorTopology) int32 {
	// Use host processor information retrieved from HCS instead of runtime.NumCPU,
	// GetMaximumProcessorCount or other OS level calls for two reasons.
	// 1. Go uses GetProcessAffinityMask and falls back to GetSystemInfo both of
	// which will not return LPs in another processor group.
	// 2. GetMaximumProcessorCount will return all processors on the system
	// but in configurations where the host partition doesn't see the full LP count
	// i.e "Minroot" scenarios this won't be sufficient.
	// (https://docs.microsoft.com/en-us/windows-server/virtualization/hyper-v/manage/manage-hyper-v-minroot-2016)
	hostCount := int32(processorTopology.LogicalProcessorCount)
	if requested > hostCount {
		log.G(ctx).WithFields(logrus.Fields{
			logfields.UVMID: uvmID,
			"requested":     requested,
			"assigned":      hostCount,
		}).Warn("Changing user requested CPUCount to current number of processors")
		return hostCount
	}
	return requested
}
