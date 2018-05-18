package uvm

// This package describes the external interface for utility VMs. These are always
// a V2 schema construct and requires RS5 or later.

import (
	"sync"

	"github.com/Microsoft/hcsshim/internal/hcs"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// UVMOptions are the set of options passed to Create() to create a utility vm.
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
	KernelDebugComPortPipe string // If kernel is in debug mode, can override the pipe here. Defaults to `\\.\pipe\vmpipe`
}

// vsmbShare is an internal structure used for ref-counting VSMB shares mapped to a utility VM.
type vsmbShare struct {
	refCount uint32
	guid     string
}

// UtilityVM is the object used by clients representing a utility VM
type UtilityVM struct {
	id              string      // Identifier for the utility VM (user supplied or generated)
	owner           string      // Owner for the utility VM (user supplied or generated)
	operatingSystem string      // "windows" or "linux"
	hcsSystem       *hcs.System // The handle to the compute system

	// VSMB shares that are mapped into a Windows UVM
	vsmbShares struct {
		sync.Mutex
		shares map[string]vsmbShare
	}

	// VPMEM devices that are mapped into a Linux UVM
	vpmemLocations struct {
		sync.Mutex
		hostPath [128]string // Limited by ACPI size.
	}

	// SCSI devices that are mapped into a Windows or Linux utility VM
	scsiLocations struct {
		sync.Mutex
		hostPath [4][64]string // Hyper-V supports 4 controllers, 64 slots per controller. Limited to 1 controller for now though.
	}

	// TODO: Plan9 will need adding for LCOW

}

// ProcessOptions are the set of options which are passed to CreateProcess() to
// create a utility vm.
type ProcessOptions struct {
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

// CreateProcess creates a process in the utility VM. This is only used
// on LCOW to run processes for remote filesystem commands, utilities, and debugging.
// It will return an error on a Windows utility VM.
//
// TODO: This could be removed as on LCOW as we could run a privileged container.
func (uvm *UtilityVM) CreateProcess(opts *ProcessOptions) error {
	return nil
}
