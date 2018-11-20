package uvm

import (
	"runtime"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// Options are the set of options passed to Create() to create a utility vm.
type Options struct {
	ID                      string                  // Identifier for the uvm. Defaults to generated GUID.
	Owner                   string                  // Specifies the owner. Defaults to executable name.
	Resources               *specs.WindowsResources // Optional resources for the utility VM. Supports Memory.limit and CPU.Count only currently. // TODO consider extending?
	AdditionHCSDocumentJSON string                  // Optional additional JSON to merge into the HCS document prior

	// Fields that can be configured via OCI annotations in runhcs.

	// Memory for UVM. Defaults to true. For physical backed memory, set to false. io.microsoft.virtualmachine.computetopology.memory.allowovercommit=true|false
	AllowOvercommit *bool

	// Memory for UVM. Defaults to false. For virtual memory with deferred commit, set to true. io.microsoft.virtualmachine.computetopology.memory.enabledeferredcommit=true|false
	EnableDeferredCommit *bool
}

// ID returns the ID of the VM's compute system.
func (uvm *UtilityVM) ID() string {
	return uvm.hcsSystem.ID()
}

// OS returns the operating system of the utility VM.
func (uvm *UtilityVM) OS() string {
	return uvm.operatingSystem
}

// Close terminates and releases resources associated with the utility VM.
func (uvm *UtilityVM) Close() error {
	uvm.Terminate()
	if uvm.gcslog != nil {
		uvm.gcslog.Close()
		uvm.gcslog = nil
	}
	err := uvm.hcsSystem.Close()
	uvm.hcsSystem = nil
	return err
}

func getMemory(r *specs.WindowsResources) int32 {
	if r == nil || r.Memory == nil || r.Memory.Limit == nil {
		return 1024 // 1GB By Default.
	}
	return int32(*r.Memory.Limit / 1024 / 1024) // OCI spec is in bytes. HCS takes MB
}

func getProcessors(r *specs.WindowsResources) int32 {
	if r == nil || r.CPU == nil || r.CPU.Count == nil {
		processors := int32(2)
		if runtime.NumCPU() == 1 {
			processors = 1
		}
		return processors
	}
	return int32(*r.CPU.Count)
}
