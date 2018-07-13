package resourcetype

// These are constants for v2 schema modify requests. These are temporary and
// may be removed from HCS ModifySettingRequest. Note these are for host-side.

// ResourceType const
const (
	Memory             = "Memory"
	CpuGroup           = "CpuGroup"
	MappedDirectory    = "MappedDirectory"
	MappedPipe         = "MappedPipe"
	MappedVirtualDisk  = "MappedVirtualDisk"
	Network            = "Network"
	VSmbShare          = "VSmbShare"
	Plan9Share         = "Plan9Share"
	CombinedLayers     = "CombinedLayers"
	HvSocket           = "HvSocket"
	SharedMemoryRegion = "SharedMemoryRegion"
	VPMemDevice        = "VPMemDevice"
	Gpu                = "Gpu"
	CosIndex           = "CosIndex" // v2.1
	Rmid               = "Rmid"     // v2.1
)
