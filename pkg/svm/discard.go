package svm

//	"github.com/Microsoft/hcsshim/internal/uvm"

func (i *instance) Discard(id string) error {
	// Keep a global service VM running - effectively a no-op
	if i.mode == ModeGlobal {
		return nil
	}

	// Write operation. Must hold the lock.
	i.Lock()
	defer i.Unlock()

	// Nothing to do if no service VMs or not found
	if i.serviceVMs == nil {
		return ErrNotFound
	}
	svmItem, exists := i.serviceVMs[id]
	if !exists {
		return ErrNotFound
	}

	// Shoot the VM
	if err := terminate(svmItem.serviceVM.utilityVM); err != nil {
		return err
	}

	delete(i.serviceVMs, id)
	return nil
}
