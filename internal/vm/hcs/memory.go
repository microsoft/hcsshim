package hcs

import (
	"github.com/Microsoft/hcsshim/internal/vm"
)

func (uvmb *utilityVMBuilder) SetMemoryLimit(memoryMB uint64) error {
	uvmb.doc.VirtualMachine.ComputeTopology.Memory.SizeInMB = memoryMB
	return nil
}

func (uvmb *utilityVMBuilder) SetMemoryConfig(config *vm.MemoryConfig) error {
	memory := uvmb.doc.VirtualMachine.ComputeTopology.Memory
	memory.AllowOvercommit = config.BackingType == vm.MemoryBackingTypeVirtual
	memory.EnableDeferredCommit = config.DeferredCommit
	memory.EnableHotHint = config.HotHint
	memory.EnableColdHint = config.ColdHint
	memory.EnableColdDiscardHint = config.ColdDiscardHint
	return nil
}

func (uvmb *utilityVMBuilder) SetMMIOConfig(lowGapMB uint64, highBaseMB uint64, highGapMB uint64) error {
	memory := uvmb.doc.VirtualMachine.ComputeTopology.Memory
	memory.LowMMIOGapInMB = lowGapMB
	memory.HighMMIOBaseInMB = highBaseMB
	memory.HighMMIOGapInMB = highGapMB
	return nil
}
