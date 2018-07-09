package hostedsettings

import "github.com/Microsoft/hcsshim/internal/schema2"

// Arguably, many of these (at least CombinedLayers) should have been generated
// by swagger.
//
// This will also change package name due to an inbound breaking change.

// This class is used by a modify request to add or remove a combined layers
// structure in the guest. For windows, the GCS applies a filter in ContainerRootPath
// using the specified layers as the parent content. Ignores property ScratchPath
// since the container path is already the scratch path. For linux, the GCS unions
// the specified layers and ScratchPath together, placing the resulting union
// filesystem at ContainerRootPath.
type CombinedLayers struct {
	ContainerRootPath string            `json:"ContainerRootPath,omitempty"`
	Layers            []hcsschema.Layer `json:"Layers,omitempty"`
	ScratchPath       string            `json:"ScratchPath,omitempty"`
}

// Defines the schema for hosted settings passed to opengcs
// TODO: These need omitempties

// SCSI. Scratch space for remote file-system commands, or R/W layer for containers
type LCOWMappedVirtualDisk struct {
	MountPath  string // /tmp/scratch for an LCOW utility VM being used as a service VM
	Lun        uint8
	Controller uint8
	ReadOnly   bool
}

// Plan 9.
type LCOWMappedDirectory struct {
	MountPath string
	Port      int32
	ShareName string // If empty not using ANames (not currently supported)
	ReadOnly  bool
}

// Read-only layers over VPMem
type LCOWMappedVPMemDevice struct {
	DeviceNumber uint32
	MountPath    string // /tmp/pN
}
