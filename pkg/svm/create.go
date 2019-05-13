package svm

import (
	"fmt"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/uvm"
)

// Create creates a service VM in this instance. In global mode, a call to
// create when the service VM is already running is a no-op. In per-instance
// mode, a second call to create will create a second service. Each
// SVM will have a scratch space (just a mount, not a container scratch)
// attached at /tmp/scratch for use by the remote filesystem utilities.
func (i *instance) Create(id string, cacheDir string, scratchDir string) error {
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
		// TODO: Do we need to increment the refcount?		svm.refCount++
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

	// Create a scratch
	if err := i.createScratchNoLock(id, DefaultScratchSizeGB, cacheDir, scratchDir); err != nil {
		svm.ComputeSystem().Terminate()
		delete(i.serviceVMs, id)
		return err
	}

	// Attach the scratch
	if _, _, err := svm.AddSCSI(filepath.Join(scratchDir, fmt.Sprintf("%s_svm_scratch.vhdx", id)), "/tmp/scratch", false); err != nil {
		svm.ComputeSystem().Terminate()
		delete(i.serviceVMs, id)
		return err
	}

	return nil
}
