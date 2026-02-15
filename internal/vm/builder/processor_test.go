//go:build windows

package builder

import (
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/vm"
)

func TestProcessorConfigAndCPUGroup(t *testing.T) {
	b, cs := newBuilder(t, vm.Linux)
	var processor ProcessorOptions = b

	processor.SetProcessorLimits(&hcsschema.VirtualMachineProcessor{Count: 4, Limit: 2500, Weight: 200})
	proc := cs.VirtualMachine.ComputeTopology.Processor
	if proc.Count != 4 || proc.Limit != 2500 || proc.Weight != 200 {
		t.Fatal("processor config not applied as expected")
	}

	processor.SetCPUGroup(&hcsschema.CpuGroup{Id: "cg1"})
	if proc.CpuGroup == nil || proc.CpuGroup.Id != "cg1" {
		t.Fatal("cpu group not applied as expected")
	}
}
