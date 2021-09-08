package uvm

import (
	"sync/atomic"
)

// ContainerCounter is used for where we layout things for a container in
// a utility VM. For WCOW it'll be C:\c\N\. For LCOW it'll be /run/gcs/c/N/.
func (uvm *UtilityVM) ContainerCounter() uint64 {
	return atomic.AddUint64(&uvm.containerCounter, 1)
}

// mountCounter is used for maintaining the number of mounts to the UVM.
// This helps in generating unique mount paths for every mount.
func (uvm *UtilityVM) UVMMountCounter() uint64 {
	return atomic.AddUint64(&uvm.mountCounter, 1)
}
