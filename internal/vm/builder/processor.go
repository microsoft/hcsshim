//go:build windows

package builder

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// ProcessorOptions configures processor settings for the Utility VM.
type ProcessorOptions interface {
	// SetProcessor sets processor related options for the Utility VM
	SetProcessor(config *hcsschema.VirtualMachineProcessor)
	// SetCPUGroup sets the CPU group that the Utility VM will belong to on a Windows host.
	SetCPUGroup(cpuGroup *hcsschema.CpuGroup)
}

var _ ProcessorOptions = (*UtilityVM)(nil)

func (uvmb *UtilityVM) SetProcessor(config *hcsschema.VirtualMachineProcessor) {
	if config != nil {
		uvmb.doc.VirtualMachine.ComputeTopology.Processor = config
	}
}

func (uvmb *UtilityVM) SetCPUGroup(cpuGroup *hcsschema.CpuGroup) {
	uvmb.doc.VirtualMachine.ComputeTopology.Processor.CpuGroup = cpuGroup
}
