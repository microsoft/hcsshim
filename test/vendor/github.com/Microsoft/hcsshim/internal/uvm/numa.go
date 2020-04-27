package uvm

import (
	"errors"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
)

// validateNUMATopology validates that there are no obvious faults with the passed
// in topology. More checks performed by the vm worker process and other layers.
func (uvm *UtilityVM) validateNUMATopology(hostTop *hcsschema.ProcessorTopology, guestTop *hcsschema.Numa) error {
	// Check if vNode count is greater than the max of 64
	if guestTop.VirtualNodeCount > MaxNUMANodeCount {
		return fmt.Errorf("invalid NUMA topology: virtual node count greater than %d", MaxNUMANodeCount)
	}
	// Check if preferred nodes is less than 0 or greater than the index of the
	// last physical node.
	index := len(hostTop.LogicalProcessors) - 1
	numNodes := hostTop.LogicalProcessors[index].NodeNumber
	for _, node := range guestTop.PreferredPhysicalNodes {
		if node < 0 || node > numNodes {
			return fmt.Errorf("invalid NUMA topology: %d is not a valid physical node index", node)
		}
	}
	// Check to see if the user passed in more settings objects than there are
	// nodes to apply them to.
	if len(guestTop.Settings) > int(guestTop.VirtualNodeCount) {
		return fmt.Errorf("invalid NUMA topology: number of settings objects exceeds virtual node count")
	}

	// Loop through NUMA settings and validate that there are no computeless or
	// memoryless nodes or less than 0 numbers. Add up memory and processor counts
	// and then verify that they match what is assigned to the UVM.
	var (
		memSize   uint64 = 0
		procCount uint32 = 0
	)
	for _, setting := range guestTop.Settings {
		if setting.VirtualSocketNumber < 0 {
			return errors.New("invalid NUMA topology: virtual socket number less than 0")
		}
		if setting.PhysicalNodeNumber < 0 || setting.PhysicalNodeNumber > uint32(numNodes) {
			return fmt.Errorf("invalid NUMA topology: %d is not a valid physical node index", setting)
		}
		if setting.CountOfMemoryBlocks == 0 {
			return errors.New("invalid NUMA topology: cannot have a memoryless node")
		}
		if setting.CountOfProcessors == 0 {
			return errors.New("invalid NUMA topology: cannot have a computeless node")
		}
		memSize += setting.CountOfMemoryBlocks
		procCount += setting.CountOfProcessors
	}

	if procCount != uint32(uvm.ProcessorCount()) {
		return fmt.Errorf("invalid NUMA topology: requested %d VPs does not match the UVMs assigned amount of %d", procCount, uvm.ProcessorCount())
	}
	if memSize != uint64(uvm.MemorySizeInMB()) {
		return fmt.Errorf("invalid NUMA topology: requested %dMBs does not match the UVMs assigned amount of %dMBs", memSize, uvm.MemorySizeInMB())
	}
	return nil
}
