package hcs

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

func (uvmb *utilityVMBuilder) SetCPUGroup(id string) error {
	uvmb.doc.VirtualMachine.ComputeTopology.Processor.CpuGroup = &hcsschema.CpuGroup{Id: id}
	return nil
}
