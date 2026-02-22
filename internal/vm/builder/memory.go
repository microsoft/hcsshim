//go:build windows

package builder

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// MemoryOptions configures memory settings for the Utility VM.
type MemoryOptions interface {
	// SetMemory sets memory related options for the Utility VM.
	SetMemory(config *hcsschema.VirtualMachineMemory)
	// SetFirmwareFallbackMeasuredSlit sets the SLIT type as "FirmwareFallbackMeasured" for the Utility VM.
	SetFirmwareFallbackMeasuredSlit()
}

var _ MemoryOptions = (*UtilityVM)(nil)

func (uvmb *UtilityVM) SetMemory(config *hcsschema.VirtualMachineMemory) {
	if config != nil {
		uvmb.doc.VirtualMachine.ComputeTopology.Memory = config
	}
}

func (uvmb *UtilityVM) SetFirmwareFallbackMeasuredSlit() {
	firmwareFallbackMeasured := hcsschema.VirtualSlitType_FIRMWARE_FALLBACK_MEASURED
	uvmb.doc.VirtualMachine.ComputeTopology.Memory.SlitType = &firmwareFallbackMeasured
}
