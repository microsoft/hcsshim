package hcs

import (
	"context"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

func (uvmb *utilityVMBuilder) SetCPUGroup(ctx context.Context, id string) error {
	uvmb.doc.VirtualMachine.ComputeTopology.Processor.CpuGroup = &hcsschema.CpuGroup{Id: id}
	return nil
}
