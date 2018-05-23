package lcowhostedsettings

// Defines the schema for hosted settings passed to opengcs

// SCSI. Scratch space for remote file-system commands, or R/W layer for containers
type MappedVirtualDisk struct {
	MountPath  string // /tmp/scratch for an LCOW utility VM being used as a service VM, OR /tmp/scratchL
	Lun        uint8
	Controller uint8
	ReadOnly   bool
}

// Plan 9.
type MappedDirectory struct {
	MountPath string // /tmp/share
	Port      int32  // 9999
	ShareName string // If empty not using ANames
	ReadOnly  bool
}

// Read-only layers over VPMem
type MappedVPMemDevice struct {
	DeviceNumber uint32
	MountPath    string // /tmp/layerN
}

//type MappedVPMemControllerV2 struct {
//	MappedDevices []MappedVPMemDeviceV2
//}

// THIS IS THE OLD ONE
// This goes in the hosted settings of a VPMem device (hand-added JJH)
//type MappedVPMemController struct {
//	MappedDevices map[int]string `json:"MappedDevices,omitempty"`
//}
