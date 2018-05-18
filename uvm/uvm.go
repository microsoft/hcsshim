package uvm

// This package describes the external interface for utility VMs. These are always
// a V2 schema construct and require RS5 or later.

import specs "github.com/opencontainers/runtime-spec/specs-go"

// UVMOptions are the set of options passed to CreateUtilityVM() to create a utility vm.
type UVMOptions struct {
	Id                      string                  // Identifier for the uvm. Defaults to generated GUID.
	Owner                   string                  // Specifies the owner. Defaults to executable name.
	OperatingSystem         string                  // "windows" or "linux".
	Resources               *specs.WindowsResources // Optional resources for the utility VM. Supports Memory.limit and CPU.Count only currently. // TODO consider extending?
	AdditionHCSDocumentJSON string                  // Optional additional JSON to merge into the HCS document prior

	// WCOW specific parameters
	LayerFolders []string // Set of folders for base layers and sandbox. Ordered from top most read-only through base read-only layer, followed by sandbox

	// LCOW specific parameters
	KirdPath               string // Folder in which kernel and initrd reside. Defaults to \Program Files\Linux Containers
	KernelFile             string // Filename under KirdPath for the kernel. Defaults to bootx64.efi
	InitrdFile             string // Filename under KirdPath for the initrd image. Defaults to initrd.img
	KernelBootOptions      string // Additional boot options for the kernel
	KernelDebugMode        bool   // Configures the kernel in debug mode using sane defaults
	KernelDebugComPortPipe string // If kernel is in debug mode, can override the pipe here.
}

// UtilityVM is the object used by clients representing a utility VM
type UtilityVM struct {
}

// CreateUtilityVM creates an HCS compute system representing a utility VM.
func CreateUtilityVM(opts *UVMOptions) (*UtilityVM, error) {
	return &UtilityVM{}, nil
}

// Start synchronously starts the utility VM.
func (uvm *UtilityVM) Start() error {
	return nil
}

// Terminate requests a utility VM terminate. If IsPending() on the error returned is true,
// it may not actually be shut down until Wait() succeeds.
func (uvm *UtilityVM) Terminate() error {
	return nil
}

// Waits synchronously waits for a utility VM to terminate.
func (uvm *UtilityVM) Wait() error {
	return nil
}

// Modifies the compute system by sending a request to HCS
func (uvm *UtilityVM) Modify(hcsModificationDocument interface{}) error {
	return nil
}

// ProcessOptions are the set of options which are passed to CreateProcess() to
// create a utility vm.
type ProcessOptions struct {
}

// CreateProcess creates a process in the utility VM. This is only used
// on LCOW to run processes for remote filesystem commands, utilities, and debugging.
// It will return an error on a Windows utility VM.
//
// TODO: This could be removed as on LCOW as we could run a privileged container.
func (uvm *UtilityVM) CreateProcess(opts *ProcessOptions) error {
	return nil
}
