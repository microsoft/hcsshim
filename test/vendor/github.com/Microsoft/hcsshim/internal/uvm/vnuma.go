//go:build windows

package uvm

import (
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/osversion"
)

// prepareVNumaTopology creates vNUMA settings for implicit (platform) or explicit (user-defined) topology.
//
// For implicit topology we look for `MaxProcessorsPerNumaNode`, `MaxSizePerNode` and `preferredNumaNodes` create options. Setting them
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
// TODO: Add exact OS build version for vNUMA support.
func prepareVNumaTopology(opts *Options) (*hcsschema.Numa, *hcsschema.NumaProcessors, error) {
	if opts.MaxProcessorsPerNumaNode == 0 && len(opts.NumaMappedPhysicalNodes) == 0 {
		// vNUMA settings are missing, return empty topology
		return nil, nil, nil
	}

	var preferredNumaNodes []int64
	for _, pn := range opts.PreferredPhysicalNumaNodes {
		preferredNumaNodes = append(preferredNumaNodes, int64(pn))
	}

	build := osversion.Get().Build
	if build < osversion.V25H1Server {
		return nil, nil, fmt.Errorf("vNUMA topology is not supported on %d version of Windows", build)
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
	return numa, nil, validate(numa)
}

const (
	wildcardPhysicalNodeNumber = 0xFF
	numaTopologyNodeCountMax   = 64
	numaChildNodeCountMax      = 64
)

// validate validates self-contained fields within the given NUMA settings.
func validate(n *hcsschema.Numa) error {
	if len(n.Settings) == 0 {
		// Nothing to validate
		return nil
	}

	var virtualNodeSet = make(map[uint32]struct{})
	var virtualSocketSet = make(map[uint32]struct{})
	var totalVPCount uint32
	var totalMemInMb uint64

	hasWildcardPhysicalNode := n.Settings[0].PhysicalNodeNumber == wildcardPhysicalNodeNumber

	for _, topology := range n.Settings {
		if topology.VirtualNodeNumber > numaChildNodeCountMax {
			return fmt.Errorf("vNUMA virtual node number %d exceeds maximum allowed value %d", topology.VirtualNodeNumber, numaChildNodeCountMax)
		}
		if topology.PhysicalNodeNumber != wildcardPhysicalNodeNumber && topology.PhysicalNodeNumber >= numaTopologyNodeCountMax {
			return fmt.Errorf("vNUMA physical node number %d exceeds maximum allowed value %d", topology.PhysicalNodeNumber, numaTopologyNodeCountMax)
		}
		if hasWildcardPhysicalNode != (topology.PhysicalNodeNumber == wildcardPhysicalNodeNumber) {
			return fmt.Errorf("vNUMA has a mix of wildcard (%d) and non-wildcard (%d) physical node numbers", wildcardPhysicalNodeNumber, topology.PhysicalNodeNumber)
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

	return nil
}

// validateNumaForVM validates the NUMA settings for a VM with the given memory settings `memorySettings`,
// processor count `procCount`, and total memory in MB `memInMb`.
func validateNumaForVM(numa *hcsschema.Numa, procCount uint32, memInMb uint64) error {
	var totalMemoryInMb uint64
	var totalProcessorCount uint32

	for _, topology := range numa.Settings {
		totalProcessorCount += topology.CountOfProcessors
		totalMemoryInMb += topology.CountOfMemoryBlocks
	}

	if (totalProcessorCount != 0) && ((totalProcessorCount) != procCount) {
		return fmt.Errorf("vNUMA total processor count %d does not match UVM processor count %d", totalProcessorCount, procCount)
	}

	if (totalMemoryInMb != 0) && (totalMemoryInMb != memInMb) {
		return fmt.Errorf("vNUMA total memory %d does not match UVM memory %d", totalMemoryInMb, memInMb)
	}
	return nil
}
