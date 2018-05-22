package uvm

// This package describes the external interface for utility VMs. These are always
// a V2 schema construct and requires RS5 or later.

import (
	"sync"

	"github.com/Microsoft/hcsshim/internal/guid"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hns"
)

//                    | WCOW | LCOW
// Container sandbox  | SCSI | SCSI
// Scratch space      | ---- | SCSI   // For file system utilities. /tmp/scratch
// Read-Only Layer    | VSMB | VPMEM
// Mapped Directory   | VSMB | PLAN9

// vsmbInfo is an internal structure used for ref-counting VSMB shares mapped to a Windows utility VM.
type vsmbInfo struct {
	refCount uint32
	guid     string
	uvmPath  string
}

// scsiInfo is an internal structure used for determining what is mapped to a utility VM.
// hostPath is required. uvmPath may be blank.
type scsiInfo struct {
	hostPath string
	uvmPath  string
}

// vpmemInfo is an internal structure used for determining VPMem devices mapped to
// a Linux utility VM.
type vpmemInfo struct {
	hostPath string
	uvmPath  string
	refCount uint32
}

type nicInfo struct {
	ID       guid.GUID
	Endpoint *hns.HNSEndpoint
}

type namespaceInfo struct {
	nics     []nicInfo
	refCount int
}

// UtilityVM is the object used by clients representing a utility VM
type UtilityVM struct {
	id              string      // Identifier for the utility VM (user supplied or generated)
	owner           string      // Owner for the utility VM (user supplied or generated)
	operatingSystem string      // "windows" or "linux"
	hcsSystem       *hcs.System // The handle to the compute system
	m               sync.Mutex

	// VSMB shares that are mapped into a Windows UVM. These are used for read-only
	// layers and mapped directories
	vsmbShares map[string]vsmbInfo

	// VPMEM devices that are mapped into a Linux UVM. These are used for read-only layers.
	vpmemDevices [maxVPMEM]vpmemInfo // Limited by ACPI size.

	// SCSI devices that are mapped into a Windows or Linux utility VM
	scsiLocations [4][64]scsiInfo // Hyper-V supports 4 controllers, 64 slots per controller. Limited to 1 controller for now though.

	// TODO: Plan9 will need adding for LCOW. These are used for mapped directories

	namespaces map[string]*namespaceInfo
}
