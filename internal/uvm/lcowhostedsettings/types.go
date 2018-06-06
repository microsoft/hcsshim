package lcowhostedsettings

// Defines the schema for hosted settings passed to opengcs
// TODO: These need omitempties

// SCSI. Scratch space for remote file-system commands, or R/W layer for containers
type MappedVirtualDisk struct {
	MountPath  string // /tmp/scratch for an LCOW utility VM being used as a service VM
	Lun        uint8
	Controller uint8
	ReadOnly   bool
}

// Plan 9.
type MappedDirectory struct {
	MountPath string
	Port      int32
	ShareName string // If empty not using ANames (not currently supported)
	ReadOnly  bool
}

// Read-only layers over VPMem
type MappedVPMemDevice struct {
	DeviceNumber uint32
	MountPath    string // /tmp/pN
}
