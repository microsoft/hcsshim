//go:build windows

package uvm

import (
	"context"
	"fmt"

	//hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/ctrdtaskapi"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func (uvm *UtilityVM) Update(ctx context.Context, data interface{}, annots map[string]string) error {
	var memoryLimitInBytes *uint64
	var processorLimits *hcsschema.ProcessorLimits

	switch resources := data.(type) {
	case *specs.WindowsResources:
		if resources.Memory != nil {
			memoryLimitInBytes = resources.Memory.Limit
		}
		if resources.CPU != nil {
			processorLimits = &hcsschema.ProcessorLimits{}
			if resources.CPU.Maximum != nil {
				processorLimits.Limit = uint64(*resources.CPU.Maximum)
			}
			if resources.CPU.Shares != nil {
				processorLimits.Weight = uint64(*resources.CPU.Shares)
			}
		}
	case *specs.LinuxResources:
		if resources.Memory != nil {
			mem := uint64(*resources.Memory.Limit)
			memoryLimitInBytes = &mem
		}
		if resources.CPU != nil {
			processorLimits = &hcsschema.ProcessorLimits{}
			if resources.CPU.Quota != nil {
				processorLimits.Limit = uint64(*resources.CPU.Quota)
			}
			if resources.CPU.Shares != nil {
				processorLimits.Weight = uint64(*resources.CPU.Shares)
			}
		}
	case *ctrdtaskapi.PolicyFragment:
		return uvm.InjectPolicyFragment(ctx, resources)
	default:
		return fmt.Errorf("invalid resource: %+v", resources)
	}

	if memoryLimitInBytes != nil {
		if err := uvm.UpdateMemory(ctx, *memoryLimitInBytes); err != nil {
			return err
		}
	}
	if processorLimits != nil {
		if err := uvm.UpdateCPULimits(ctx, processorLimits); err != nil {
			return err
		}
	}

	// Check if an annotation was sent to update cpugroup membership
	if cpuGroupID, ok := annots[annotations.CPUGroupID]; ok {
		if err := uvm.SetCPUGroup(ctx, cpuGroupID); err != nil {
			return err
		}
	}

	// check if annotation for mounting volumes was added
	/*
		if annots["mount"] != "" {
			mountPaths := strings.Split(annots["mount"], "::")
			// make sure there is host and container path
			if len(mountPaths) != 2 {
				return fmt.Errorf("host and container path not specified")
			}

			log.G(ctx).Debug("mountPaths[0] %v mountPaths[1] %v", mountPaths[0], mountPaths[1])
			settings := hcsschema.MappedDirectory{
				HostPath:      mountPaths[0],
				ContainerPath: mountPaths[1],
				ReadOnly:      true,
			}
			if err := uvm.requestAddContainerMount(ctx, resourcepaths.VSMBShareResourcePath, settings); err != nil {
				return err
			}
		}
	*/
	return nil
}
