package resourcepaths

//nolint:deadcode,varcheck
const (
	GPUResourcePath                  string = "VirtualMachine/ComputeTopology/Gpu"
	MemoryResourcePath               string = "VirtualMachine/ComputeTopology/Memory/SizeInMB"
	CPUGroupResourcePath             string = "VirtualMachine/ComputeTopology/Processor/CpuGroup"
	IdledResourcePath                string = "VirtualMachine/ComputeTopology/Processor/IdledProcessors"
	CPUFrequencyPowerCapResourcePath string = "VirtualMachine/ComputeTopology/Processor/CpuFrequencyPowerCap"
	CPULimitsResourcePath            string = "VirtualMachine/ComputeTopology/Processor/Limits"
	SerialResourceFormat             string = "VirtualMachine/Devices/ComPorts/%d"
	FlexibleIovResourceFormat        string = "VirtualMachine/Devices/FlexibleIov/%s"
	LicensingResourcePath            string = "VirtualMachine/Devices/Licensing"
	MappedPipeResourceFormat         string = "VirtualMachine/Devices/MappedPipes/%s"
	NetworkResourceFormat            string = "VirtualMachine/Devices/NetworkAdapters/%s"
	Plan9ShareResourcePath           string = "VirtualMachine/Devices/Plan9/Shares"
	SCSIResourceFormat               string = "VirtualMachine/Devices/Scsi/%s/Attachments/%d"
	SharedMemoryRegionResourcePath   string = "VirtualMachine/Devices/SharedMemory/Regions"
	VirtualPCIResourceFormat         string = "VirtualMachine/Devices/VirtualPci/%s"
	VPMemControllerResourceFormat    string = "VirtualMachine/Devices/VirtualPMem/Devices/%d"
	VPMemDeviceResourceFormat        string = "VirtualMachine/Devices/VirtualPMem/Devices/%d/Mappings/%d"
	VSMBShareResourcePath            string = "VirtualMachine/Devices/VirtualSmb/Shares"
	HvSocketConfigResourceFormat     string = "VirtualMachine/Devices/HvSocket/HvSocketConfig/ServiceTable/%s"
)
