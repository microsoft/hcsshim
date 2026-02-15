//go:build windows

package builder

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// ProcessorOptions configures processor settings for the Utility VM.
type ProcessorOptions interface {
	// SetProcessorLimits applies Count, Limit, and Weight from the provided config.
	SetProcessorLimits(config *hcsschema.VirtualMachineProcessor)
	// SetCPUGroup sets the CPU group that the Utility VM will belong to on a Windows host.
	SetCPUGroup(cpuGroup *hcsschema.CpuGroup)
}

var _ ProcessorOptions = (*UtilityVM)(nil)

func (uvmb *UtilityVM) SetProcessorLimits(config *hcsschema.VirtualMachineProcessor) {
	processor := uvmb.doc.VirtualMachine.ComputeTopology.Processor
	processor.Count = config.Count
	processor.Limit = config.Limit
	processor.Weight = config.Weight
}

func (uvmb *UtilityVM) SetCPUGroup(cpuGroup *hcsschema.CpuGroup) {
	uvmb.doc.VirtualMachine.ComputeTopology.Processor.CpuGroup = cpuGroup
}
