//go:build windows

package uvm

import (
	"io"
	"net"
	"sync"

	"github.com/Microsoft/go-winio/pkg/guid"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	"github.com/Microsoft/hcsshim/internal/hns"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
)

//                    | WCOW | LCOW
// Container scratch  | SCSI | SCSI
// Scratch space      | ---- | SCSI   // For file system utilities. /tmp/scratch
// Read-Only Layer    | VSMB | VPMEM
// Mapped Directory   | VSMB | PLAN9

type nicInfo struct {
	ID       string
	Endpoint *hns.HNSEndpoint
}

type namespaceInfo struct {
	nics map[string]*nicInfo
}

// UtilityVM is the object used by clients representing a utility VM
type UtilityVM struct {
	id               string               // Identifier for the utility VM (user supplied or generated)
	runtimeID        guid.GUID            // Hyper-V VM ID
	owner            string               // Owner for the utility VM (user supplied or generated)
	operatingSystem  string               // "windows" or "linux"
	hcsSystem        *hcs.System          // The handle to the compute system
	gcListener       net.Listener         // The GCS connection listener
	gc               *gcs.GuestConnection // The GCS connection
	processorCount   int32
	physicallyBacked bool       // If the uvm is backed by physical memory and not virtual memory
	m                sync.Mutex // Lock for adding/removing devices

	exitErr error
	exitCh  chan struct{}

	// devicesPhysicallyBacked indicates if additional devices added to a uvm should be
	// entirely physically backed
	devicesPhysicallyBacked bool

	// GCS bridge protocol and capabilities
	protocol  uint32
	guestCaps schema1.GuestDefinedCapabilities

	// containerCounter is the current number of containers that have been
	// created. This is never decremented in the life of the UVM.
	//
	// NOTE: All accesses to this MUST be done atomically.
	containerCounter uint64

	// noWritableFileShares disables mounting any writable vSMB or Plan9 shares
	// on the uVM. This prevents containers in the uVM modifying files and directories
	// made available via the "mounts" options in the container spec, or shared
	// to the uVM directly.
	// This option does not prevent writable SCSI mounts.
	noWritableFileShares bool

	// VSMB shares that are mapped into a Windows UVM. These are used for read-only
	// layers and mapped directories.
	// We maintain two sets of maps, `vsmbDirShares` tracks shares that are
	// unrestricted mappings of directories. `vsmbFileShares` tracks shares that
	// are restricted to some subset of files in the directory. This is used as
	// part of a temporary fix to allow WCOW single-file mapping to function.
	vsmbDirShares   map[string]*VSMBShare
	vsmbFileShares  map[string]*VSMBShare
	vsmbCounter     uint64 // Counter to generate a unique share name for each VSMB share.
	vsmbNoDirectMap bool   // indicates if VSMB devices should be added with the `NoDirectMap` option

	// VPMEM devices that are mapped into a Linux UVM. These are used for read-only layers, or for
	// booting from VHD.
	vpmemMaxCount           uint32 // The max number of VPMem devices.
	vpmemMaxSizeBytes       uint64 // The max size of the layer in bytes per vPMem device.
	vpmemMultiMapping       bool   // Enable mapping multiple VHDs onto a single VPMem device
	vpmemDevicesDefault     [MaxVPMEMCount]*vPMemInfoDefault
	vpmemDevicesMultiMapped [MaxVPMEMCount]*vPMemInfoMulti

	// SCSI devices that are mapped into a Windows or Linux utility VM
	SCSIManager         *scsi.Manager
	scsiControllerCount uint32 // Number of SCSI controllers in the utility VM
	reservedSCSISlots   []scsi.Slot

	encryptScratch bool                          // Enable scratch encryption
	vpciDevices    map[VPCIDeviceKey]*VPCIDevice // map of device instance id to vpci device

	// Plan9 are directories mapped into a Linux utility VM
	plan9Counter uint64 // Each newly-added plan9 share has a counter used as its ID in the ResourceURI and for the name

	namespaces map[string]*namespaceInfo

	outputListener       net.Listener
	outputProcessingDone chan struct{}
	outputHandler        OutputHandler

	entropyListener net.Listener

	// Handle to the vmmem process associated with this UVM. Used to look up
	// memory metrics for the UVM.
	vmmemProcess windows.Handle
	// Tracks the error returned when looking up the vmmem process.
	vmmemErr error
	// We only need to look up the vmmem process once, then we keep a handle
	// open.
	vmmemOnce sync.Once

	// mountCounter is the number of mounts that have been added to the UVM
	// This is used in generating a unique mount path inside the UVM for every mount.
	// Access to this variable should be done atomically.
	mountCounter uint64

	// Location that container process dumps will get written too.
	processDumpLocation string

	// The CreateOpts used to create this uvm. These can be either of type
	// uvm.OptionsLCOW or uvm.OptionsWCOW
	createOpts interface{}

	// Network config proxy client. If nil then this wasn't requested and the
	// uvms network will be configured locally.
	ncProxyClientAddress string

	// networkSetup handles the logic for setting up and tearing down any network configuration
	// for the Utility VM.
	networkSetup NetworkSetup

	// noInheritHostTimezone specifies whether to not inherit the hosts timezone for the UVM. UTC will be set as the default instead.
	// This only applies for WCOW.
	noInheritHostTimezone bool

	// confidentialUVMOptions hold confidential UVM specific options
	confidentialUVMOptions *ConfidentialOptions
}

func (uvm *UtilityVM) ScratchEncryptionEnabled() bool {
	return uvm.encryptScratch
}

// OutputHandler is used to process the output from the program run in the UVM.
type OutputHandler func(io.Reader)

type OutputHandlerCreator func(*Options) OutputHandler
