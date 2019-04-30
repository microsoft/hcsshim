package svm

import (
	"fmt"
	"sync"

	"github.com/Microsoft/hcsshim/internal/uvm"
)

type serviceVM struct {
	utilityVM *uvm.UtilityVM
	// sync.Mutex                     // Serialises operations being performed in this service VM.
	// scratchAttached bool           // Has a scratch been attached?
	// config          *client.Config // Represents the service VM item.

	// // Indicates that the vm is started
	// startStatus chan interface{}
	// startError  error

	// // Indicates that the vm is stopped
	// stopStatus chan interface{}
	// stopError  error

	// attachCounter uint64                  // Increasing counter for each add
	// attachedVHDs  map[string]*attachedVHD // Map ref counting all the VHDS we've hot-added/hot-removed.
	// unionMounts   map[string]int          // Map ref counting all the union filesystems we mounted.
}

// serviceVMItem is our internal structure representing an item in our
// map of service VMs we are maintaining.
type serviceVMItem struct {
	serviceVM *serviceVM // actual service vm object
	// Not quite sure what this was used for yet.... refCount  int        // refcount for VM
}

// instance is the internal representation of an Instance of service VMs being
// managed.
type instance struct {
	// Lock to control writes to this instance
	sync.Mutex

	// mode is the mode in which this instance is operating.
	mode Mode

	// serviceVMs is a map of all the service VMs this instance is managing.
	// It represents an ID to service VM mapping.
	serviceVMs map[string]serviceVMItem
}

// NewOptions defines the parameters passed to New().
type NewOptions struct {
	Mode Mode
}

// New creates a new instances of a managed (set of) service VMs.
func New(opts *NewOptions) (Instance, error) {
	if opts == nil {
		return nil, fmt.Errorf("options cannot be nil")
	}
	if opts.Mode != ModeUnique && opts.Mode != ModeGlobal {
		return nil, fmt.Errorf("invalid mode %d", opts.Mode)
	}
	return &instance{mode: opts.Mode}, nil
}
