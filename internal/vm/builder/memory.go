//go:build windows

package builder

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// MemoryOptions configures memory settings for the Utility VM.
type MemoryOptions interface {
	// SetMemoryLimit sets the amount of memory in megabytes that the Utility VM will be assigned.
	SetMemoryLimit(memoryMB uint64)
	// SetMemoryHints sets memory hint settings for the Utility VM.
	SetMemoryHints(config *hcsschema.VirtualMachineMemory)
	// SetMMIOConfig sets memory mapped IO configurations for the Utility VM.
	SetMMIOConfig(lowGapMB uint64, highBaseMB uint64, highGapMB uint64)
	// SetFirmwareFallbackMeasuredSlit sets the SLIT type as "FirmwareFallbackMeasured" for the Utility VM.
	SetFirmwareFallbackMeasuredSlit()
}

var _ MemoryOptions = (*UtilityVM)(nil)

func (uvmb *UtilityVM) SetMemoryLimit(memoryMB uint64) {
	uvmb.doc.VirtualMachine.ComputeTopology.Memory.SizeInMB = memoryMB
}

func (uvmb *UtilityVM) SetMemoryHints(config *hcsschema.VirtualMachineMemory) {
	if config == nil {
		return
	}

	memory := uvmb.doc.VirtualMachine.ComputeTopology.Memory
	if config.Backing != nil {
		memory.Backing = config.Backing
		memory.AllowOvercommit = *config.Backing == hcsschema.MemoryBackingType_VIRTUAL
	}
	memory.EnableDeferredCommit = config.EnableDeferredCommit
	memory.EnableHotHint = config.EnableHotHint
	memory.EnableColdHint = config.EnableColdHint
	memory.EnableColdDiscardHint = config.EnableColdDiscardHint
}

func (uvmb *UtilityVM) SetMMIOConfig(lowGapMB uint64, highBaseMB uint64, highGapMB uint64) {
	memory := uvmb.doc.VirtualMachine.ComputeTopology.Memory
	memory.LowMMIOGapInMB = lowGapMB
	memory.HighMMIOBaseInMB = highBaseMB
	memory.HighMMIOGapInMB = highGapMB
}

func (uvmb *UtilityVM) SetFirmwareFallbackMeasuredSlit() {
	firmwareFallbackMeasured := hcsschema.VirtualSlitType_FIRMWARE_FALLBACK_MEASURED
	uvmb.doc.VirtualMachine.ComputeTopology.Memory.SlitType = &firmwareFallbackMeasured
}
