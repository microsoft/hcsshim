package privileged

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/processorinfo"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

// This file contains helpers for converting parts of the oci spec to useful
// structures/limits to be applied to a job object.

// Oci spec to job object limit information.
func specToLimits(ctx context.Context, s *specs.Spec) *jobLimits {
	var (
		cpuCount     int32
		cpuWeight    uint32
		cpuRate      uint32
		memLimit     uint64
		maxIops      int64
		maxBandwidth int64
	)

	// Cpu limits
	cpuCount = oci.ParseAnnotationsCPUCount(ctx, s, oci.AnnotationContainerProcessorCount, 0)
	cpuRate = uint32(oci.ParseAnnotationsCPULimit(ctx, s, oci.AnnotationContainerProcessorLimit, 0))
	cpuWeight = uint32(oci.ParseAnnotationsCPUWeight(ctx, s, oci.AnnotationContainerProcessorWeight, 0))

	// Todo (dcantah): CpuWeight complains when it's set. Can only set rate for some reason

	hostCPUCount := processorinfo.ProcessorCount()
	if cpuCount > hostCPUCount {
		log.G(ctx).WithFields(logrus.Fields{
			"requested": cpuCount,
			"assigned":  hostCPUCount,
		}).Warn("Changing user requested CPUCount to current number of processors")
		cpuCount = hostCPUCount
	}

	// Memory limits
	memLimit = uint64(oci.ParseAnnotationsMemory(ctx, s, oci.AnnotationContainerMemorySizeInMB, 0))

	// IO limits
	maxBandwidth = int64(oci.ParseAnnotationsStorageBps(ctx, s, oci.AnnotationContainerStorageQoSBandwidthMaximum, 0))
	maxIops = int64(oci.ParseAnnotationsStorageIops(ctx, s, oci.AnnotationContainerStorageQoSIopsMaximum, 0))

	return &jobLimits{
		cpuRate:        cpuRate,
		cpuWeight:      cpuWeight,
		affinity:       uintptr(cpuCount),
		maxIops:        maxIops,
		maxBandwidth:   maxBandwidth,
		jobMemoryLimit: uintptr(memLimit),
	}
}
