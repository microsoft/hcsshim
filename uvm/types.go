package uvm

// This package describes the external interface for utility VMs. These are always
// a V2 schema construct and requires RS5 or later.

import (
	"sync"

	"github.com/Microsoft/hcsshim/internal/hcs"
)

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
		hostPath [maxVPMEM]string // Limited by ACPI size.
	}

	// SCSI devices that are mapped into a Windows or Linux utility VM
	scsiLocations struct {
		sync.Mutex
		hostPath [4][64]string // Hyper-V supports 4 controllers, 64 slots per controller. Limited to 1 controller for now though.
	}

	// TODO: Plan9 will need adding for LCOW

}
