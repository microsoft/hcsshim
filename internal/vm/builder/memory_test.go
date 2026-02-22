//go:build windows

package builder

import (
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

func TestMemoryConfig(t *testing.T) {
	b, cs := newBuilder(t)
	var memory MemoryOptions = b

	backingVirtual := hcsschema.MemoryBackingType_VIRTUAL

	memory.SetMemory(&hcsschema.VirtualMachineMemory{
		SizeInMB:              512,
		Backing:               &backingVirtual,
		EnableDeferredCommit:  true,
		EnableHotHint:         true,
		EnableColdHint:        false,
		EnableColdDiscardHint: true,
		LowMMIOGapInMB:        64,
		HighMMIOBaseInMB:      128,
		HighMMIOGapInMB:       256,
	})

	mem := cs.VirtualMachine.ComputeTopology.Memory
	if mem.SizeInMB != 512 {
		t.Fatalf("SizeInMB = %d, want %d", mem.SizeInMB, 512)
	}
	if mem.Backing == nil || *mem.Backing != hcsschema.MemoryBackingType_VIRTUAL {
		t.Fatal("Backing not set to VIRTUAL")
	}
	if !mem.EnableDeferredCommit || !mem.EnableHotHint || mem.EnableColdHint || !mem.EnableColdDiscardHint {
		t.Fatal("memory hints not applied as expected")
	}
	if mem.LowMMIOGapInMB != 64 || mem.HighMMIOBaseInMB != 128 || mem.HighMMIOGapInMB != 256 {
		t.Fatal("MMIO config not applied as expected")
	}

	backingPhysical := hcsschema.MemoryBackingType_PHYSICAL
	memory.SetMemory(&hcsschema.VirtualMachineMemory{
		SizeInMB:              1024,
		Backing:               &backingPhysical,
		EnableDeferredCommit:  true,
		EnableHotHint:         true,
		EnableColdHint:        false,
		EnableColdDiscardHint: true,
	})

	mem = cs.VirtualMachine.ComputeTopology.Memory
	if mem.SizeInMB != 1024 {
		t.Fatalf("SizeInMB = %d, want %d", mem.SizeInMB, 1024)
	}
	if mem.Backing == nil || *mem.Backing != hcsschema.MemoryBackingType_PHYSICAL {
		t.Fatal("Backing not set to PHYSICAL")
	}
	if !mem.EnableDeferredCommit || !mem.EnableHotHint || mem.EnableColdHint || !mem.EnableColdDiscardHint {
		t.Fatal("memory hints not applied as expected")
	}

	memory.SetFirmwareFallbackMeasuredSlit()
	if mem.SlitType == nil || *mem.SlitType != hcsschema.VirtualSlitType_FIRMWARE_FALLBACK_MEASURED {
		t.Fatal("SlitType not set to FIRMWARE_FALLBACK_MEASURED")
	}
}

func TestMemoryNilConfig(t *testing.T) {
	b, cs := newBuilder(t)
	var memory MemoryOptions = b

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetMemory panicked: %v", r)
		}
	}()

	memory.SetMemory(nil)
	memory.SetMemory(&hcsschema.VirtualMachineMemory{})

	mem := cs.VirtualMachine.ComputeTopology.Memory
	if mem.Backing != nil {
		t.Fatal("Backing should remain nil when not provided")
	}
}
