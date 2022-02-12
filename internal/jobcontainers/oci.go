package jobcontainers

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/jobobject"
	"github.com/Microsoft/hcsshim/internal/processorinfo"
	"github.com/Microsoft/hcsshim/pkg/annotations"

	"github.com/Microsoft/hcsshim/internal/oci"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// This file contains helpers for converting parts of the oci spec to useful
// structures/limits to be applied to a job object.

const processorWeightMax = 10000

// inheritUserTokenIsSet checks if the annotation that specifies whether we should inherit the token of the current process is set.
func inheritUserTokenIsSet(annots map[string]string) bool {
	return annots[annotations.HostProcessInheritUser] == "true"
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
	memLimitMB := oci.ParseAnnotationsMemory(ctx, s, annotations.ContainerMemorySizeInMB, 0)

	// IO limits
	maxBandwidth := int64(oci.ParseAnnotationsStorageBps(ctx, s, annotations.ContainerStorageQoSBandwidthMaximum, 0))
	maxIops := int64(oci.ParseAnnotationsStorageIops(ctx, s, annotations.ContainerStorageQoSIopsMaximum, 0))

	return &jobobject.JobLimits{
		CPULimit:           realCPULimit,
		CPUWeight:          realCPUWeight,
		MaxIOPS:            maxIops,
		MaxBandwidth:       maxBandwidth,
		MemoryLimitInBytes: memLimitMB * 1024 * 1024,
	}, nil
}

// calculateJobCPUWeight converts processor cpu weight to job object cpu weight.
//
// `processorWeight` is the processor cpu weight to convert.
func calculateJobCPUWeight(processorWeight uint32) uint32 {
	if processorWeight == 0 {
		return 0
	}
	return 1 + uint32((8*processorWeight)/processorWeightMax)
}

// calculateJobCPURate converts processor cpu count to job object cpu rate.
//
// `hostProcs` is the total host's processor count.
// `processorCount` is the processor count to convert to cpu rate.
func calculateJobCPURate(hostProcs uint32, processorCount uint32) uint32 {
	rate := (processorCount * 10000) / hostProcs
	if rate == 0 {
		return 1
	}
	return rate
}
