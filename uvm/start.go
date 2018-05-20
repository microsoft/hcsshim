package uvm

// Start synchronously starts the utility VM.
func (uvm *UtilityVM) Start() error {
	return uvm.hcsSystem.Start()
}
