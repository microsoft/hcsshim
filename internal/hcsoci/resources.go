package hcsoci

import (
	"context"
	"fmt"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/ctrdtaskapi"
)

// NormalizeProcessorCount returns the `Min(requested, logical CPU count)`.
func NormalizeProcessorCount(ctx context.Context, cid string, requestedCount, hostCount int32) int32 {
	if requestedCount > hostCount {
		log.G(ctx).WithFields(logrus.Fields{
			"id":              cid,
			"requested count": requestedCount,
			"assigned count":  hostCount,
		}).Warn("Changing user requested cpu count to current number of processors on the host")
		return hostCount
	} else {
		return requestedCount
	}
}

// NormalizeMemorySize returns the requested memory size in MB aligned up to an even number
func NormalizeMemorySize(ctx context.Context, cid string, requestedSizeMB uint64) uint64 {
	actualMB := (requestedSizeMB + 1) &^ 1 // align up to an even number
	if requestedSizeMB != actualMB {
		log.G(ctx).WithFields(logrus.Fields{
			"id":          cid,
			"requestedMB": requestedSizeMB,
			"actualMB":    actualMB,
		}).Warn("Changing user requested MemorySizeInMB to align to 2MB")
	}
	return actualMB
}

// NormalizeUVMUpdateResourcesRequest accepts an unmarshalled [spec.UpdateTaskRequest.Resources]
// and the accompanying annotations and returns the Resources struct with the appropriate fields
// updated based on the VM resource annotations.
func NormalizeUVMUpdateResourcesRequest(ctx context.Context, data interface{}, annots map[string]string) (interface{}, error) {
	// calling this directly in uvm.Update() would cause an import cycle, so the best alternative is to
	// call from containerd-shim-runhcs
	return normalizeUpdateResourcesRequest(ctx, false, data, annots)
}

// NormalizeContainerUpdateResourcesRequest is similar to [NormalizeUVMUpdateResourcesRequest],
// except it uses container resource annotations.
func NormalizeContainerUpdateResourcesRequest(ctx context.Context, data interface{}, annots map[string]string) (interface{}, error) {
	return normalizeUpdateResourcesRequest(ctx, true, data, annots)
}

func normalizeUpdateResourcesRequest(ctx context.Context, isContainer bool, data interface{}, annots map[string]string) (interface{}, error) {
	var (
		memorySizeInMB  = annotations.MemorySizeInMB
		processorCount  = annotations.ProcessorCount
		processorLimit  = annotations.ProcessorLimit
		processorWeight = annotations.ProcessorWeight
	)
	if isContainer {
		memorySizeInMB = annotations.ContainerMemorySizeInMB
		processorCount = annotations.ContainerProcessorCount
		processorLimit = annotations.ContainerProcessorLimit
		processorWeight = annotations.ContainerProcessorWeight
	}

	// could use [ConvertCPULimits], but that would only work for containers, and wouldn't distinguish between
	// an annotation being set to "0", and the annotation not being present.
	// And would still need to write out logic for uVM CPU annotation parsing.
	//
	// Don't bother checking if multiple CPU values are set for containers, since hcsTask will check in [updateWCOWResources].
	switch resources := data.(type) {
	case *specs.WindowsResources:
		if x := oci.ParseAnnotationsUint64(ctx, annots, memorySizeInMB, 0); x != 0 {
			if resources.Memory == nil {
				resources.Memory = &specs.WindowsMemoryResources{}
			}
			if y := resources.Memory.Limit; y != nil && *y > 0 {
				return nil, fmt.Errorf("conflicting memory limits between resource limit (%d) and %q annotation (%d)", *y, memorySizeInMB, x)
			}
			xx := x * memory.MiB
			resources.Memory.Limit = &xx
		}

		if x := oci.ParseAnnotationsUint64(ctx, annots, processorCount, 0); x != 0 {
			if resources.CPU == nil {
				resources.CPU = &specs.WindowsCPUResources{}
			}

			if y := resources.CPU.Count; y != nil && *y > 0 {
				return nil, fmt.Errorf("conflicting CPU count between resource limit (%d) and %q annotation (%d)", *y, processorCount, x)
			}
			resources.CPU.Count = &x
		}
		if x := oci.ParseAnnotationsUint64(ctx, annots, processorLimit, 0); x != 0 {
			if resources.CPU == nil {
				resources.CPU = &specs.WindowsCPUResources{}
			}
			if y := resources.CPU.Maximum; y != nil && *y > 0 {
				return nil, fmt.Errorf("conflicting CPU maximum between resource limit (%d) and %q annotation (%d)", *y, processorLimit, x)
			}
			xx := uint16(x)
			resources.CPU.Maximum = &xx
		}
		if x := oci.ParseAnnotationsUint64(ctx, annots, processorWeight, 0); x != 0 {
			if resources.CPU == nil {
				resources.CPU = &specs.WindowsCPUResources{}
			}
			if y := resources.CPU.Shares; y != nil && *y > 0 {
				return nil, fmt.Errorf("conflicting CPU shares between resource limit (%d) and %q annotation (%d)", *y, processorWeight, x)
			}
			xx := uint16(x)
			resources.CPU.Shares = &xx
		}
		return resources, nil
	case *specs.LinuxResources:
		if x := oci.ParseAnnotationsUint64(ctx, annots, memorySizeInMB, 0); x != 0 {
			if resources.Memory == nil {
				resources.Memory = &specs.LinuxMemory{}
			}
			if y := resources.Memory.Limit; y != nil && *y > 0 {
				return nil, fmt.Errorf("conflicting memory limits between resource limit (%d) and %q annotation (%d)", *y, memorySizeInMB, x)
			}
			xx := int64(x * memory.MiB)
			resources.Memory.Limit = &xx
		}

		if x := oci.ParseAnnotationsUint64(ctx, annots, processorLimit, 0); x != 0 {
			if resources.CPU == nil {
				resources.CPU = &specs.LinuxCPU{}
			}
			if y := resources.CPU.Quota; y != nil && *y > 0 {
				return nil, fmt.Errorf("conflicting CPU maximum between resource limit (%d) and %q annotation (%d)", *y, processorLimit, x)
			}
			xx := int64(x)
			resources.CPU.Quota = &xx
		}
		if x := oci.ParseAnnotationsUint64(ctx, annots, processorWeight, 0); x != 0 {
			if resources.CPU == nil {
				resources.CPU = &specs.LinuxCPU{}
			}
			if y := resources.CPU.Shares; y != nil && *y > 0 {
				return nil, fmt.Errorf("conflicting CPU shares between resource limit (%d) and %q annotation (%d)", *y, processorWeight, x)
			}
			resources.CPU.Shares = &x
		}
		return resources, nil
	case *ctrdtaskapi.PolicyFragment:
		return data, nil
	default:
	}
	return nil, fmt.Errorf("invalid resource: %+v", data)
}
