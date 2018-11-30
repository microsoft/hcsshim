package uvm

// Waits synchronously waits for a utility VM to terminate.
func (uvm *UtilityVM) Wait() error {
	<-uvm.gcsLogsExited
	return nil
	// return uvm.hcsSystem.Wait()
}
