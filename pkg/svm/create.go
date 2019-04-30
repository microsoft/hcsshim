package svm

import (
	"github.com/Microsoft/hcsshim/internal/uvm"
)

func (i *instance) Create(id string) error {
	if i.mode == ModeGlobal {
		id = globalID
	}

	// Write operation. Must hold the lock.
	i.Lock()
	defer i.Unlock()

	// Make sure the map of service VMs is initialised
	if i.serviceVMs == nil {
		i.serviceVMs = make(map[string]serviceVMItem)
	}

	// Nothing to do if the service VM already exists
	if _, exists := i.serviceVMs[id]; exists {
		// TODO: Do we need to inrement the refcount?		svm.refCount++
		return nil
	}

	// Get the default set of options based on the ID (may now be the global ID).
	opts := uvm.NewDefaultOptionsLCOW(id, "")

	// Create the utility VM
	svm, err := uvm.CreateLCOW(opts)
	if err != nil {
		return err
	}

	// Start it.
	if err := svm.Start(); err != nil {
		return err
	}

	svmItem := serviceVMItem{
		// TODO: Is this needed? 		refCount: 1,
		serviceVM: &serviceVM{
			utilityVM: svm,
		},
	}
	i.serviceVMs[id] = svmItem

	return nil
}
