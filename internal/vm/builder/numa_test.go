//go:build windows

package builder

import (
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/vm"
)

func TestNUMASettings(t *testing.T) {
	b, cs := newBuilder(t, vm.Linux)
	var numaManager NumaOptions = b
	virtualNodeCount := uint8(2)
	maxSizePerNode := uint64(4096)
	numaProcessors := &hcsschema.NumaProcessors{
		CountPerNode:  hcsschema.Range{Max: 4},
		NodePerSocket: 1,
	}
	numa := &hcsschema.Numa{
		VirtualNodeCount:       virtualNodeCount,
		PreferredPhysicalNodes: []int64{0, 1},
		Settings: []hcsschema.NumaSetting{
			{
				VirtualNodeNumber:   0,
				PhysicalNodeNumber:  0,
				VirtualSocketNumber: 0,
				CountOfProcessors:   2,
				CountOfMemoryBlocks: 1024,
				MemoryBackingType:   hcsschema.MemoryBackingType_VIRTUAL,
			},
			{
				VirtualNodeNumber:   1,
				PhysicalNodeNumber:  1,
				VirtualSocketNumber: 1,
				CountOfProcessors:   2,
				CountOfMemoryBlocks: 1024,
				MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
			},
		},
		MaxSizePerNode: maxSizePerNode,
	}

	numaManager.SetNUMAProcessorsSettings(numaProcessors)
	numaManager.SetNUMASettings(numa)

	gotProcessors := cs.VirtualMachine.ComputeTopology.Processor.NumaProcessorsSettings
	if gotProcessors == nil {
		t.Fatal("NUMA processor settings not applied")
	}
	if gotProcessors.CountPerNode.Max != 4 {
		t.Fatalf("CountPerNode.Max = %d, want %d", gotProcessors.CountPerNode.Max, 4)
	}
	if gotProcessors.NodePerSocket != 1 {
		t.Fatalf("NodePerSocket = %d, want %d", gotProcessors.NodePerSocket, 1)
	}

	gotNUMA := cs.VirtualMachine.ComputeTopology.Numa
	if gotNUMA == nil {
		t.Fatal("NUMA settings not applied")
	}
	if gotNUMA.VirtualNodeCount != virtualNodeCount {
		t.Fatalf("VirtualNodeCount = %d, want %d", gotNUMA.VirtualNodeCount, virtualNodeCount)
	}
	if gotNUMA.MaxSizePerNode != maxSizePerNode {
		t.Fatalf("MaxSizePerNode = %d, want %d", gotNUMA.MaxSizePerNode, maxSizePerNode)
	}
	if len(gotNUMA.PreferredPhysicalNodes) != 2 || gotNUMA.PreferredPhysicalNodes[0] != 0 || gotNUMA.PreferredPhysicalNodes[1] != 1 {
		t.Fatalf("PreferredPhysicalNodes = %v, want [0 1]", gotNUMA.PreferredPhysicalNodes)
	}
	if len(gotNUMA.Settings) != 2 {
		t.Fatalf("Settings length = %d, want %d", len(gotNUMA.Settings), 2)
	}

	first := gotNUMA.Settings[0]
	if first.VirtualNodeNumber != 0 || first.PhysicalNodeNumber != 0 || first.VirtualSocketNumber != 0 {
		t.Fatal("first NUMA setting node/socket numbers not applied as expected")
	}
	if first.CountOfProcessors != 2 || first.CountOfMemoryBlocks != 1024 {
		t.Fatal("first NUMA setting processor/memory counts not applied as expected")
	}
	if first.MemoryBackingType != hcsschema.MemoryBackingType_VIRTUAL {
		t.Fatalf("first NUMA setting MemoryBackingType = %s, want %s", first.MemoryBackingType, hcsschema.MemoryBackingType_VIRTUAL)
	}

	second := gotNUMA.Settings[1]
	if second.VirtualNodeNumber != 1 || second.PhysicalNodeNumber != 1 || second.VirtualSocketNumber != 1 {
		t.Fatal("second NUMA setting node/socket numbers not applied as expected")
	}
	if second.CountOfProcessors != 2 || second.CountOfMemoryBlocks != 1024 {
		t.Fatal("second NUMA setting processor/memory counts not applied as expected")
	}
	if second.MemoryBackingType != hcsschema.MemoryBackingType_PHYSICAL {
		t.Fatalf("second NUMA setting MemoryBackingType = %s, want %s", second.MemoryBackingType, hcsschema.MemoryBackingType_PHYSICAL)
	}
}
