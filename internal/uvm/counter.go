package uvm

import "sync"

var (
	m       sync.Mutex
	counter uint64
)

// ContainerCounter is used for LCOW. It's where we mount the overlay filesystem
// inside the utility VM when mounting container layers. eg /tmp/cN
func (uvm *UtilityVM) ContainerCounter() uint64 {
	m.Lock()
	defer m.Unlock()
	counter++
	return counter
}
