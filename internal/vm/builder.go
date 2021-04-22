package vm

import (
	"context"
)

type UVMBuilder interface {
	// Create will create the Utility VM in a paused/powered off state with whatever is present in the implementation
	// of the interfaces config at the time of the call.
	Create(ctx context.Context) (UVM, error)
}

type MemoryBackingType uint8

const (
	MemoryBackingTypeVirtual MemoryBackingType = iota
	MemoryBackingTypePhysical
)

// MemoryConfig holds the memory options that should be configurable for a Utility VM.
type MemoryConfig struct {
	BackingType     MemoryBackingType
	DeferredCommit  bool
	HotHint         bool
	ColdHint        bool
	ColdDiscardHint bool
}

// MemoryManager handles setting and managing memory configurations for the Utility VM.
type MemoryManager interface {
	// SetMemoryLimit sets the amount of memory in megabytes that the Utility VM will be assigned.
	SetMemoryLimit(memoryMB uint64) error
	// SetMemoryConfig sets an array of different memory configuration options available. This includes things like the
	// type of memory to back the VM (virtual/physical).
	SetMemoryConfig(config *MemoryConfig) error
	// SetMMIOConfig sets memory mapped IO configurations for the Utility VM.
	SetMMIOConfig(lowGapMB uint64, highBaseMB uint64, highGapMB uint64) error
}

// ProcessorManager handles setting and managing processor configurations for the Utility VM.
type ProcessorManager interface {
	// SetProcessorCount sets the number of virtual processors that will be assigned to the Utility VM.
	SetProcessorCount(count uint32) error
}

// SerialManager manages setting up serial consoles for the Utility VM.
type SerialManager interface {
	// SetSerialConsole sets up a serial console for `port`. Output will be relayed to the listener specified
	// by `listenerPath`. For HCS `listenerPath` this is expected to be a path to a named pipe.
	SetSerialConsole(port uint32, listenerPath string) error
}

// BootManager manages boot configurations for the Utility VM.
type BootManager interface {
	// SetUEFIBoot sets UEFI configurations for booting a Utility VM.
	SetUEFIBoot(dir string, path string, args string) error
	// SetLinuxKernelDirectBoot sets Linux direct boot configurations for booting a Utility VM.
	SetLinuxKernelDirectBoot(kernel string, initRD string, cmd string) error
}

// StorageQosManager manages setting storage limits on the Utility VM.
type StorageQosManager interface {
	// SetStorageQos sets storage related options for the Utility VM
	SetStorageQos(iopsMaximum int64, bandwidthMaximum int64) error
}

// WindowsConfigManager manages options specific to a Windows host (cpugroups etc.)
type WindowsConfigManager interface {
	// SetCPUGroup sets the CPU group that the Utility VM will belong to on a Windows host.
	SetCPUGroup(id string) error
}

// LinuxConfigManager manages options specific to a Linux host.
type LinuxConfigManager interface{}
