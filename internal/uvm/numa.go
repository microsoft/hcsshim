package uvm

import (
	"errors"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/osversion"
)

func (uvm *UtilityVM) setupNUMA(defaultNUMA bool, hostTop *hcsschema.ProcessorTopology, guestTop *hcsschema.Numa) (*hcsschema.Numa, error) {
	// ProcessorTopology includes the logical processor count and the node mappings
	// of LPs sequentially. By reading the last entries 'NodeNumber' field
	// (0 indexed) and adding 1 we can get the hosts node count.
	index := len(hostTop.LogicalProcessors) - 1
	numNodes := hostTop.LogicalProcessors[index].NodeNumber + 1
	if defaultNUMA && guestTop == nil {
		return &hcsschema.Numa{
			VirtualNodeCount: numNodes,
		}, nil
	} else if guestTop != nil {
		// If settings were provided validate the topology passed in or error if
		// before the build that supports this.
		if len(guestTop.Settings) != 0 {
			if osversion.Get().Build < 18943 {
				return nil, errors.New("Per virtual node settings are not supported on builds older than 18943")
			}
			if err := uvm.validateNUMATopology(hostTop, guestTop); err != nil {
				return nil, fmt.Errorf("NUMA topology validation failed: %s", err)
			}
		}
	}
	return guestTop, nil
}

// validateNUMATopology validates that there are no obvious faults with the passed
// in topology. More checks performed by the vm worker process and other layers.
func (uvm *UtilityVM) validateNUMATopology(hostTop *hcsschema.ProcessorTopology, guestTop *hcsschema.Numa) error {
	// Check if vNode count is greater than the max of 64
	if guestTop.VirtualNodeCount > MaxNUMANodeCount {
		return fmt.Errorf("invalid NUMA topology: virtual node count greater than %d", MaxNUMANodeCount)
	}
	// Check if preferred nodes are greater than the index of the
	// last physical node.
	index := len(hostTop.LogicalProcessors) - 1
	numNodes := hostTop.LogicalProcessors[index].NodeNumber
	for _, node := range guestTop.PreferredPhysicalNodes {
		if node > numNodes {
			return fmt.Errorf("invalid NUMA topology: %d is not a valid physical node index", node)
		}
	}
	// Loop through NUMA settings and validate that there are no computeless or
	// memoryless nodes. Add up memory and processor counts and then verify that
	// they match what is assigned to the UVM.
	var (
		memSize   uint64 = 0
		procCount uint32 = 0
	)
	for _, setting := range guestTop.Settings {
		if setting.PhysicalNodeNumber > uint32(numNodes) {
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
