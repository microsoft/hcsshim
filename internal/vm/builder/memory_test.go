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
	backingPhysical := hcsschema.MemoryBackingType_PHYSICAL

	memory.SetMemoryLimit(512)
	memory.SetMemoryHints(&hcsschema.VirtualMachineMemory{
		Backing:               &backingVirtual,
		EnableDeferredCommit:  true,
		EnableHotHint:         true,
		EnableColdHint:        false,
		EnableColdDiscardHint: true,
	})

	mem := cs.VirtualMachine.ComputeTopology.Memory
	if mem.SizeInMB != 512 {
		t.Fatalf("SizeInMB = %d, want %d", mem.SizeInMB, 512)
	}
	if mem.Backing == nil || *mem.Backing != hcsschema.MemoryBackingType_VIRTUAL {
		t.Fatal("Backing not set to VIRTUAL")
	}
	if !mem.AllowOvercommit || !mem.EnableDeferredCommit || !mem.EnableHotHint || mem.EnableColdHint || !mem.EnableColdDiscardHint {
		t.Fatal("memory hints not applied as expected")
	}

	memory.SetMemoryHints(&hcsschema.VirtualMachineMemory{
		Backing:               &backingPhysical,
		EnableDeferredCommit:  true,
		EnableHotHint:         true,
		EnableColdHint:        false,
		EnableColdDiscardHint: true,
	})

	mem = cs.VirtualMachine.ComputeTopology.Memory
	if mem.Backing == nil || *mem.Backing != hcsschema.MemoryBackingType_PHYSICAL {
		t.Fatal("Backing not set to PHYSICAL")
	}
	if mem.AllowOvercommit || !mem.EnableDeferredCommit || !mem.EnableHotHint || mem.EnableColdHint || !mem.EnableColdDiscardHint {
		t.Fatal("memory hints not applied as expected")
	}

	memory.SetMMIOConfig(64, 128, 256)
	if mem.LowMMIOGapInMB != 64 || mem.HighMMIOBaseInMB != 128 || mem.HighMMIOGapInMB != 256 {
		t.Fatal("MMIO config not applied as expected")
	}

	memory.SetFirmwareFallbackMeasuredSlit()
	if mem.SlitType == nil || *mem.SlitType != hcsschema.VirtualSlitType_FIRMWARE_FALLBACK_MEASURED {
		t.Fatal("SlitType not set to FIRMWARE_FALLBACK_MEASURED")
	}
}

func TestMemoryHintsNilBacking(t *testing.T) {
	b, cs := newBuilder(t)
	var memory MemoryOptions = b

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetMemoryHints panicked: %v", r)
		}
	}()

	memory.SetMemoryHints(nil)
	memory.SetMemoryHints(&hcsschema.VirtualMachineMemory{})

	mem := cs.VirtualMachine.ComputeTopology.Memory
	if mem.Backing != nil {
		t.Fatal("Backing should remain nil when not provided")
	}
	if mem.AllowOvercommit {
		t.Fatal("AllowOvercommit should remain false when Backing is nil")
	}
}
