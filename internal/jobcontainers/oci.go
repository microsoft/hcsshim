package jobcontainers

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/processorinfo"

	"github.com/Microsoft/hcsshim/internal/jobobject"

	"github.com/Microsoft/hcsshim/internal/oci"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// This file contains helpers for converting parts of the oci spec to useful
// structures/limits to be applied to a job object.
func calculateJobCPUWeight(processorWeight uint32) uint32 {
	if processorWeight == 0 {
		return 0
	}
	return 1 + uint32((8*processorWeight)/jobobject.CPUWeightMax)
}

func calculateJobCPURate(hostProcs uint32, processorCount uint32) uint32 {
	rate := (processorCount * 10000) / hostProcs
	if rate == 0 {
		return 1
	}
	return rate
}

func getUserTokenInheritAnnotation(annotations map[string]string) bool {
	val, ok := annotations[oci.AnnotationHostProcessInheritUser]
	return ok && val == "true"
}

// Oci spec to job object limit information. Will do any conversions to job object specific values from
// their respective OCI representations. E.g. we convert CPU count into the correct job object cpu
// rate value internally.
func specToLimits(ctx context.Context, cid string, s *specs.Spec) (*jobobject.JobLimits, error) {
	hostCPUCount := processorinfo.ProcessorCount()
	cpuCount, cpuLimit, cpuWeight, err := hcsoci.ConvertCPULimits(ctx, cid, s, hostCPUCount)
	if err != nil {
		return nil, err
	}

	realCPULimit, realCPUWeight := uint32(cpuLimit), uint32(cpuWeight)
	if cpuCount != 0 {
		// Job object API does not support "CPU count". Instead, we translate the notion of "count" into
		// CPU limit, which represents the amount of the host system's processors that the job can use to
		// a percentage times 100. For example, to let the job use 20% of the available LPs the rate would
		// be 20 times 100, or 2,000.
		realCPULimit = calculateJobCPURate(uint32(hostCPUCount), uint32(cpuCount))
	} else if cpuWeight != 0 {
		realCPUWeight = calculateJobCPUWeight(realCPUWeight)
	}

	// Memory limit
	memLimitMB := oci.ParseAnnotationsMemory(ctx, s, oci.AnnotationContainerMemorySizeInMB, 0)

	// IO limits
	maxBandwidth := int64(oci.ParseAnnotationsStorageBps(ctx, s, oci.AnnotationContainerStorageQoSBandwidthMaximum, 0))
	maxIops := int64(oci.ParseAnnotationsStorageIops(ctx, s, oci.AnnotationContainerStorageQoSIopsMaximum, 0))

	return &jobobject.JobLimits{
		CPULimit:           realCPULimit,
		CPUWeight:          realCPUWeight,
		MaxIOPS:            maxIops,
		MaxBandwidth:       maxBandwidth,
		MemoryLimitInBytes: memLimitMB * 1024 * 1024,
	}, nil
}
