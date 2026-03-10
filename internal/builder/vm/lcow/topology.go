//go:build windows

package lcow

import (
	"context"
	"fmt"

	runhcsoptions "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/processorinfo"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	"github.com/Microsoft/hcsshim/osversion"
	shimannotations "github.com/Microsoft/hcsshim/pkg/annotations"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/sirupsen/logrus"
)

// parseCPUOptions parses CPU options from annotations and options.
func parseCPUOptions(ctx context.Context, opts *runhcsoptions.Options, annotations map[string]string) (*hcsschema.VirtualMachineProcessor, error) {
	log.G(ctx).Debug("parseCPUOptions: starting CPU options parsing")

	count := oci.ParseAnnotationsInt32(ctx, annotations, shimannotations.ProcessorCount, opts.VmProcessorCount)
	if count <= 0 {
		count = vmutils.DefaultProcessorCountForUVM()
	}

	processorTopology, err := processorinfo.HostProcessorInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host processor information: %w", err)
	}
	// To maintain compatibility with Docker and older shim we need to automatically downgrade
	// a user CPU count if the setting is not possible.
	count = vmutils.NormalizeProcessorCount(ctx, "", count, processorTopology)

	limit := oci.ParseAnnotationsInt32(ctx, annotations, shimannotations.ProcessorLimit, 0)
	weight := oci.ParseAnnotationsInt32(ctx, annotations, shimannotations.ProcessorWeight, 0)

	// Set CPU configuration based on parsed values.
	cpu := &hcsschema.VirtualMachineProcessor{
		Count:  uint32(count),
		Limit:  uint64(uint32(limit)),
		Weight: uint64(uint32(weight)),
	}

	// CPU group configuration
	cpuGroupID := oci.ParseAnnotationsString(annotations, shimannotations.CPUGroupID, "")
	if cpuGroupID != "" && osversion.Build() < osversion.V21H1 {
		return nil, vmutils.ErrCPUGroupCreateNotSupported
	}
	cpu.CpuGroup = &hcsschema.CpuGroup{Id: cpuGroupID}

	// Resource Partition ID parsing.
	resourcePartitionID := oci.ParseAnnotationsString(annotations, shimannotations.ResourcePartitionID, "")
	if resourcePartitionID != "" {
		log.G(ctx).WithField("resourcePartitionID", resourcePartitionID).Debug("setting resource partition ID")

		if _, err = guid.FromString(resourcePartitionID); err != nil {
			return nil, fmt.Errorf("failed to parse resource_partition_id %q to GUID: %w", resourcePartitionID, err)
		}

		// CPU group and resource partition are mutually exclusive.
		if cpuGroupID != "" {
			return nil, fmt.Errorf("cpu_group_id and resource_partition_id cannot be set at the same time")
		}
	}

	log.G(ctx).WithFields(logrus.Fields{
		"processorCount":  count,
		"processorLimit":  limit,
		"processorWeight": weight,
		"cpuGroupID":      cpuGroupID,
	}).Debug("parseCPUOptions completed successfully")

	return cpu, nil
}

// parseMemoryOptions parses memory options from annotations and options.
func parseMemoryOptions(ctx context.Context, opts *runhcsoptions.Options, annotations map[string]string, isFullyPhysicallyBacked bool) (*hcsschema.VirtualMachineMemory, error) {
	log.G(ctx).Debug("parseMemoryOptions: starting memory options parsing")
	mem := &hcsschema.VirtualMachineMemory{}

	mem.SizeInMB = oci.ParseAnnotationsUint64(ctx, annotations, shimannotations.MemorySizeInMB, uint64(opts.VmMemorySizeInMb))
	if mem.SizeInMB <= 0 {
		mem.SizeInMB = 1024
	}
	// Normalize memory size to be a multiple of 256MB, as required by Hyper-V.
	mem.SizeInMB = vmutils.NormalizeMemorySize(ctx, "", mem.SizeInMB)

	mem.LowMMIOGapInMB = oci.ParseAnnotationsUint64(ctx, annotations, shimannotations.MemoryLowMMIOGapInMB, 0)
	mem.HighMMIOBaseInMB = oci.ParseAnnotationsUint64(ctx, annotations, shimannotations.MemoryHighMMIOBaseInMB, 0)
	mem.HighMMIOGapInMB = oci.ParseAnnotationsUint64(ctx, annotations, shimannotations.MemoryHighMMIOGapInMB, 0)

	mem.AllowOvercommit = oci.ParseAnnotationsBool(ctx, annotations, shimannotations.AllowOvercommit, true)
	mem.EnableDeferredCommit = oci.ParseAnnotationsBool(ctx, annotations, shimannotations.EnableDeferredCommit, false)

	if isFullyPhysicallyBacked {
		mem.AllowOvercommit = false
	}

	mem.EnableColdDiscardHint = oci.ParseAnnotationsBool(ctx, annotations, shimannotations.EnableColdDiscardHint, false)
	if mem.EnableColdDiscardHint && osversion.Build() < 18967 {
		return nil, fmt.Errorf("EnableColdDiscardHint is not supported on builds older than 18967")
	}

	if mem.EnableDeferredCommit && !mem.AllowOvercommit {
		return nil, fmt.Errorf("enable_deferred_commit is not supported on physically backed vms")
	}

	log.G(ctx).WithFields(logrus.Fields{
		"memorySizeMB":         mem.SizeInMB,
		"allowOvercommit":      mem.AllowOvercommit,
		"fullyPhysicalBacked":  isFullyPhysicallyBacked,
		"enableDeferredCommit": mem.EnableDeferredCommit,
		"enableColdDiscard":    mem.EnableColdDiscardHint,
	}).Debug("parseMemoryOptions completed successfully")

	return mem, nil
}

// parseNUMAOptions parses NUMA options from annotations and uses vmutils to
// prepare the vNUMA topology.
func parseNUMAOptions(ctx context.Context, annotations map[string]string, cpuCount uint32, memorySize uint64, allowOvercommit bool) (*hcsschema.Numa, *hcsschema.NumaProcessors, error) {
	log.G(ctx).Debug("parseNUMAOptions: starting NUMA options parsing")

	// Build vmutils.NumaConfig from annotations
	numaOpts := &vmutils.NumaConfig{
		MaxProcessorsPerNumaNode:   oci.ParseAnnotationsUint32(ctx, annotations, shimannotations.NumaMaximumProcessorsPerNode, 0),
		MaxMemorySizePerNumaNode:   oci.ParseAnnotationsUint64(ctx, annotations, shimannotations.NumaMaximumMemorySizePerNode, 0),
		PreferredPhysicalNumaNodes: oci.ParseAnnotationCommaSeparatedUint32(ctx, annotations, shimannotations.NumaPreferredPhysicalNodes, []uint32{}),
		NumaMappedPhysicalNodes:    oci.ParseAnnotationCommaSeparatedUint32(ctx, annotations, shimannotations.NumaMappedPhysicalNodes, []uint32{}),
		NumaProcessorCounts:        oci.ParseAnnotationCommaSeparatedUint32(ctx, annotations, shimannotations.NumaCountOfProcessors, []uint32{}),
		NumaMemoryBlocksCounts:     oci.ParseAnnotationCommaSeparatedUint64(ctx, annotations, shimannotations.NumaCountOfMemoryBlocks, []uint64{}),
	}

	// Use vmutils to prepare the vNUMA topology.
	hcsNuma, hcsNumaProcessors, err := vmutils.PrepareVNumaTopology(ctx, numaOpts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to prepare vNUMA topology: %w", err)
	}

	if hcsNuma != nil {
		log.G(ctx).WithField("virtualNodeCount", hcsNuma.VirtualNodeCount).Debug("vNUMA topology configured")
		if allowOvercommit {
			return nil, nil, fmt.Errorf("vNUMA supports only Physical memory backing type")
		}
		if err := vmutils.ValidateNumaForVM(hcsNuma, cpuCount, memorySize); err != nil {
			return nil, nil, fmt.Errorf("failed to validate vNUMA settings: %w", err)
		}
	}

	log.G(ctx).WithFields(logrus.Fields{
		"numa":           hcsNuma,
		"numaProcessors": hcsNumaProcessors,
	}).Debug("parseNUMAOptions completed successfully")

	return hcsNuma, hcsNumaProcessors, nil
}
