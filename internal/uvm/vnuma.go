//go:build windows

package uvm

import (
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// prepareVNumaTopology creates vNUMA settings for implicit (platform) or explicit (user-defined) topology.
//
// For implicit topology we look for `MaxProcessorsPerNumaNode` and `MaxSizePerNode` create options. Setting them
// in HCS doc, will trigger platform to create vNUMA topology based on the given values. Based on experiments, the
// platform will create an evenly distributed topology based on requested memory and processor count for the HCS VM.
//
// For explicit topology we look for `NumaMappedPhysicalNodes`, `NumaProcessorCounts` and `NumaMemoryBlocksCounts` create
// options. The above options are number slices, where a value at index `i` in each slice represents the corresponding
// value for the `i`th vNUMA node.
// Limitations:
// - only hcsschema.MemoryBackingType_PHYSICAL is supported
// - `PhysicalNumaNodes` values at index `i` will be mapped to virtual node number `i`
// - client is responsible for setting wildcard physical node numbers
//
// TODO: We also assume that `hcsschema.Numa.PreferredPhysicalNodes` can be used for implicit placement as well as
// for explicit placement in the case when all wildcard physical nodes are present.
func prepareVNumaTopology(opts *Options) (*hcsschema.Numa, *hcsschema.NumaProcessors, error) {
	if opts.MaxProcessorsPerNumaNode == 0 && len(opts.NumaMappedPhysicalNodes) == 0 {
		// vNUMA settings are missing, return empty topology
		return nil, nil, nil
	}

	var preferredNumaNodes []int64
	for _, pn := range opts.PreferredPhysicalNumaNodes {
		preferredNumaNodes = append(preferredNumaNodes, int64(pn))
	}

	// Implicit vNUMA topology.
	if opts.MaxProcessorsPerNumaNode > 0 {
		if opts.MaxSizePerNode == 0 {
			return nil, nil, fmt.Errorf("max size per node must be set when max processors per numa node is set")
		}
		numaProcessors := &hcsschema.NumaProcessors{
			CountPerNode: hcsschema.Range{
				Max: opts.MaxProcessorsPerNumaNode,
			},
		}
		numa := &hcsschema.Numa{
			MaxSizePerNode:         opts.MaxSizePerNode,
			PreferredPhysicalNodes: preferredNumaNodes,
		}
		return numa, numaProcessors, nil
	}

	// Explicit vNUMA topology.

	numaNodeCount := len(opts.NumaMappedPhysicalNodes)
	if numaNodeCount != len(opts.NumaProcessorCounts) || numaNodeCount != len(opts.NumaMemoryBlocksCounts) {
		return nil, nil, fmt.Errorf("mismatch in number of physical numa nodes and the corresponding processor and memory blocks count")
	}

	numa := &hcsschema.Numa{
		VirtualNodeCount:       uint8(numaNodeCount),
		Settings:               []hcsschema.NumaSetting{},
		PreferredPhysicalNodes: preferredNumaNodes,
	}
	for i := 0; i < numaNodeCount; i++ {
		nodeTopology := hcsschema.NumaSetting{
			VirtualNodeNumber:   uint32(i),
			PhysicalNodeNumber:  opts.NumaMappedPhysicalNodes[i],
			VirtualSocketNumber: uint32(i),
			MemoryBackingType:   hcsschema.MemoryBackingType_PHYSICAL,
			CountOfProcessors:   opts.NumaProcessorCounts[i],
			CountOfMemoryBlocks: opts.NumaMemoryBlocksCounts[i],
		}
		numa.Settings = append(numa.Settings, nodeTopology)
	}
	return numa, nil, Validate(numa)
}

const (
	WildcardPhysicalNodeNumber = 0xFF
	NumaTopologyNodeCountMax   = 64
	NumaChildNodeCountMax      = 64
)

// Validate validates self-contained fields within the given NUMA settings.
//
// TODO (maksiman): Check if we need to add compute-less node validation. For now, assume that it's supported.
func Validate(n *hcsschema.Numa) error {
	if len(n.Settings) == 0 {
		// Nothing to validate
		return nil
	}

	var virtualNodeSet = make(map[uint32]struct{})
	var virtualSocketSet = make(map[uint32]struct{})
	var totalVPCount uint32
	var totalMemInMb uint64
	var highestVNodeNumber uint32
	var highestVSocketNumber uint32

	hasWildcardPhysicalNode := n.Settings[0].PhysicalNodeNumber == WildcardPhysicalNodeNumber

	for _, topology := range n.Settings {
		if topology.VirtualNodeNumber > NumaChildNodeCountMax {
			return fmt.Errorf("vNUMA virtual node number %d exceeds maximum allowed value %d", topology.VirtualNodeNumber, NumaChildNodeCountMax)
		}
		if topology.PhysicalNodeNumber != WildcardPhysicalNodeNumber && topology.PhysicalNodeNumber >= NumaTopologyNodeCountMax {
			return fmt.Errorf("vNUMA physical node number %d exceeds maximum allowed value %d", topology.PhysicalNodeNumber, NumaTopologyNodeCountMax)
		}
		if hasWildcardPhysicalNode != (topology.PhysicalNodeNumber == WildcardPhysicalNodeNumber) {
			return fmt.Errorf("vNUMA has a mix of wildcard (%d) and non-wildcard (%d) physical node numbers", WildcardPhysicalNodeNumber, topology.PhysicalNodeNumber)
		}

		if topology.CountOfMemoryBlocks == 0 {
			return fmt.Errorf("vNUMA nodes with no memory are not allowed")
		}

		totalVPCount += topology.CountOfProcessors
		totalMemInMb += topology.CountOfMemoryBlocks

		if _, ok := virtualNodeSet[topology.VirtualNodeNumber]; ok {
			return fmt.Errorf("vNUMA virtual node number %d is duplicated", topology.VirtualNodeNumber)
		}
		virtualNodeSet[topology.VirtualNodeNumber] = struct{}{}

		if topology.MemoryBackingType != hcsschema.MemoryBackingType_PHYSICAL && topology.MemoryBackingType != hcsschema.MemoryBackingType_VIRTUAL {
			return fmt.Errorf("vNUMA memory backing type %s is invalid", topology.MemoryBackingType)
		}

		if highestVNodeNumber < topology.VirtualNodeNumber {
			highestVNodeNumber = topology.VirtualNodeNumber
		}
		if highestVSocketNumber < topology.VirtualSocketNumber {
			highestVSocketNumber = topology.VirtualSocketNumber
		}

		virtualSocketSet[topology.VirtualSocketNumber] = struct{}{}
	}

	// Either both total memory and processor count should be zero or both should be non-zero
	if (totalMemInMb == 0) != (totalVPCount == 0) {
		return fmt.Errorf("partial resource allocation is not allowed")
	}

	// At least
	if totalMemInMb == 0 && hasWildcardPhysicalNode {
		return fmt.Errorf("completely empty topology is not allowed")
	}

	if len(virtualNodeSet) != int(highestVNodeNumber+1) {
		return fmt.Errorf("holes in vNUMA node numbers are not allowed")
	}

	if len(virtualSocketSet) != int(highestVSocketNumber+1) {
		return fmt.Errorf("holes in vNUMA socket numbers are not allowed")
	}
	return nil
}

// ValidateNumaForVM validates the NUMA settings for a VM with the given memory settings `memorySettings`,
// processor count `procCount`, and total memory in MB `memInMb`.
func ValidateNumaForVM(numa *hcsschema.Numa, vmMemoryBackingType hcsschema.MemoryBackingType, procCount uint32, memInMb uint64) error {
	var hasVirtuallyBackedNode, hasPhysicallyBackedNode bool
	var totalMemoryInMb uint64
	var totalProcessorCount uint32

	for _, topology := range numa.Settings {
		if topology.MemoryBackingType != vmMemoryBackingType && topology.MemoryBackingType != hcsschema.MemoryBackingType_HYBRID {
			return fmt.Errorf("vNUMA memory backing type %s does not match UVM memory backing type %s", topology.MemoryBackingType, vmMemoryBackingType)
		}
		if topology.MemoryBackingType == hcsschema.MemoryBackingType_PHYSICAL {
			hasPhysicallyBackedNode = true
		}
		if topology.MemoryBackingType == hcsschema.MemoryBackingType_VIRTUAL {
			hasVirtuallyBackedNode = true
		}
		totalProcessorCount += topology.CountOfProcessors
		totalMemoryInMb += topology.CountOfMemoryBlocks
	}

	if vmMemoryBackingType == hcsschema.MemoryBackingType_HYBRID {
		if !hasVirtuallyBackedNode || !hasPhysicallyBackedNode {
			return fmt.Errorf("vNUMA must have both physically and virtually backed nodes for UVM with hybrid memory")
		}
	}

	if (totalProcessorCount != 0) && ((totalProcessorCount) != procCount) {
		return fmt.Errorf("vNUMA total processor count %d does not match UVM processor count %d", totalProcessorCount, procCount)
	}

	if (totalMemoryInMb != 0) && (totalMemoryInMb != memInMb) {
		return fmt.Errorf("vNUMA total memory %d does not match UVM memory %d", totalMemoryInMb, memInMb)
	}
	return nil
}
