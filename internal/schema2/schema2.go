// +build windows

package schema2

import (
	"github.com/Microsoft/hcsshim/internal/schemaversion"
)

// This file contains the structures necessary to call the HCS in v2 schema format.
// NOTE: The v2 schema is in flux and under development as at March 2018.
// Requires RS5+

type GuestOsV2 struct {
	HostName string `json:"HostName,omitempty"`
}

type ContainersResourcesLayerV2 struct {
	Id    string `json:"Id,omitempty"`
	Path  string `json:"Path,omitempty"`
	Cache string `json:"Cache,omitempty"` //  Unspecified defaults to Enabled
}

type ContainersResourcesStorageQoSV2 struct {
	IOPSMaximum      uint64 `json:"IOPSMaximum,omitempty"`
	BandwidthMaximum uint64 `json:"BandwidthMaximum,omitempty"`
}

type ContainersResourcesStorageV2 struct {
	// List of layers that describe the parent hierarchy for a container's
	// storage. These layers combined together, presented as a disposable
	// and/or committable working storage, are used by the container to
	// record all changes done to the parent layers.
	Layers []ContainersResourcesLayerV2 `json:"Layers,omitempty"`

	// Path that points to the scratch space of a container, where parent
	// layers are combined together to present a new disposable and/or
	// committable layer with the changes done during its runtime.
	Path string `json:"Path,omitempty"`

	StorageQoS *ContainersResourcesStorageQoSV2 `json:"StorageQoS,omitempty"`
}

type ContainersResourcesMappedDirectoryV2 struct {
	HostPath          string `json:"HostPath,omitempty"`
	ContainerPath     string `json:"ContainerPath,omitempty"`
	ReadOnly          bool   `json:"ReadOnly,omitempty"`
	Lun               uint8  `json:"Lun,omitempty"`
	AttachOnly        bool   `json:AttachOnly,omitempty"`        // If `true` then not mapped to the ContainerPath. This is used, for instance, if the disk doesn't yet have a filesystem on it
	OverwriteIfExists bool   `json:OverwriteIfExists,omitempty"` // If `true` then delete `ContainerPath` if it exists. Only used if the container path will be a volume mount point and is not a drive letter. Otherwise this parameter is silently ignored.
	CacheMode         string `json:CacheMode,omitempty"`         // Unspecified defaults to cache just the parent VHDs
}

type ContainersResourcesMappedPipeV2 struct {
	ContainerPipeName string `json:"ContainerPipeName,omitempty"`
	HostPath          string `json:"HostPath,omitempty"`
}

type ContainersResourcesMemoryV2 struct {
	Maximum uint64 `json:"Maximum,omitempty"`
}

type ContainersResourcesProcessorV2 struct {
	Count   uint32 `json:"Count,omitempty"`
	Maximum uint64 `json:"Maximum,omitempty"`
	Weight  uint64 `json:"Weight,omitempty"`
}

type ContainersResourcesNetworkingV2 struct {
	AllowUnqualifiedDnsQuery   bool     `json:"AllowUnqualifiedDnsQuery,omitempty"`
	DNSSearchList              string   `json:"DNSSearchList,omitempty"`
	NetworkSharedContainerName string   `json:"NetworkSharedContainerName,omitempty"`
	Namespace                  string   `json:"Namespace,omitempty"`       //  Guid in windows; string in linux
	NetworkAdapters            []string `json:"NetworkAdapters,omitempty"` // JJH Query. Guid in schema.containers.resources.mars
}

type HvSocketServiceConfigV2 struct {
	//  SDDL string that HvSocket will check before allowing a host process to bind  to this specific service.
	// If not specified, defaults to the system DefaultBindSecurityDescriptor.
	BindSecurityDescriptor string `json:"BindSecurityDescriptor,omitempty"`

	//  SDDL string that HvSocket will check before allowing a host process to connect
	// to this specific service.  If not specified, defaults to the system DefaultConnectSecurityDescriptor.
	ConnectSecurityDescriptor string `json:"ConnectSecurityDescriptor,omitempty"`

	//  If true, HvSocket will process wildcard binds for this service/system combination.
	// Wildcard binds are secured in the registry at  SOFTWARE/Microsoft/Windows NT/CurrentVersion/Virtualization/HvSocket/WildcardDescriptors
	AllowWildcardBinds bool `json:"AllowWildcardBinds,omitempty"`

	Disabled bool `json:"Disabled,omitempty"`
}

type HvSocketSystemConfigV2 struct {
	//  SDDL string that HvSocket will check before allowing a host process to bind  to an unlisted service for this specific container/VM (not wildcard binds).
	DefaultBindSecurityDescriptor string `json:"DefaultBindSecurityDescriptor,omitempty"`

	//  SDDL string that HvSocket will check before allowing a host process to connect  to an unlisted service in the VM/container.
	DefaultConnectSecurityDescriptor string `json:"DefaultConnectSecurityDescriptor,omitempty"`

	ServiceTable map[string]HvSocketServiceConfigV2 `json:"ServiceTable,omitempty"`
}

type ContainersResourcesHvSocketV2 struct {
	Config                 *HvSocketSystemConfigV2 `json:"Config,omitempty"`
	EnablePowerShellDirect bool                    `json:"EnablePowerShellDirect,omitempty"`
	EnableUtcRelay         bool                    `json:"EnableUtcRelay,omitempty"`
	EnableAuditing         bool                    `json:"EnableAuditing,omitempty"`
}

type RegistryKeyV2 struct {
	Hive     string `json:"Hive,omitempty"`
	Name     string `json:"Name,omitempty"`
	Volatile bool   `json:"Volatile,omitempty"`
}

type RegistryValueV2 struct {
	// JJH Check the types in this structure
	Key         *RegistryKeyV2 `json:"Key,omitempty"`
	Name        string         `json:"Name,omitempty"`
	Type        string         `json:"Type,omitempty"`
	StringValue string         `json:"StringValue,omitempty"` //  One and only one value type must be set.
	BinaryValue string         `json:"BinaryValue,omitempty"`
	DWordValue  int32          `json:"DWordValue,omitempty"`
	QWordValue  int32          `json:"QWordValue,omitempty"`
	CustomType  int32          `json:"CustomType,omitempty"` //  Only used if RegistryValueType is CustomType  The data is in BinaryValue
}

type RegistryChangesV2 struct {
	AddValues  []RegistryValueV2 `json:"AddValues,omitempty"`
	DeleteKeys []RegistryKeyV2   `json:"DeleteKeys,omitempty"`
}

type ContainerV2 struct {
	GuestOS           *GuestOsV2                             `json:"GuestOS,omitempty"`
	Storage           *ContainersResourcesStorageV2          `json:"Storage,omitempty"`
	MappedDirectories []ContainersResourcesMappedDirectoryV2 `json:"MappedDirectories,omitempty"`
	MappedPipes       []ContainersResourcesMappedPipeV2      `json:"MappedPipes,omitempty"`
	Memory            *ContainersResourcesMemoryV2           `json:"Memory,omitempty"`
	Processor         *ContainersResourcesProcessorV2        `json:"Processor,omitempty"`
	Networking        *ContainersResourcesNetworkingV2       `json:"Networking,omitempty"`
	HvSocket          *ContainersResourcesHvSocketV2         `json:"HvSocket,omitempty"`
	RegistryChanges   *RegistryChangesV2                     `json:"RegistryChanges,omitempty"`
}

type HostedSystemV2 struct {
	SchemaVersion *schemaversion.SchemaVersion `json:"SchemaVersion,omitempty"`
	Container     *ContainerV2                 `json:"Container,omitempty"`
}

type VirtualMachinesResourcesUefiBootEntryV2 struct {
	UefiDevice   string `json:"uefi_device,omitempty"`
	DevicePath   string `json:"device_path,omitempty"`
	DiskNumber   int32  `json:"disk_number,omitempty"`
	OptionalData string `json:"optional_data,omitempty"`
}

type VirtualMachinesResourcesUefiV2 struct {
	EnableDebugger       bool                                     `json:"EnableDebugger,omitempty"`
	SecureBootTemplateId string                                   `json:"SecureBootTemplateId,omitempty"`
	BootThis             *VirtualMachinesResourcesUefiBootEntryV2 `json:"BootThis,omitempty"`
	Console              string                                   `json:"Console,omitempty"`
}

type VirtualMachinesResourcesChipsetV2 struct {
	UEFI                  *VirtualMachinesResourcesUefiV2 `json:"UEFI,omitempty"`
	IsNumLockDisabled     bool                            `json:"IsNumLockDisabled,omitempty"`
	BaseBoardSerialNumber string                          `json:"BaseBoardSerialNumber,omitempty"`
	ChassisSerialNumber   string                          `json:"ChassisSerialNumber,omitempty"`
	ChassisAssetTag       string                          `json:"ChassisAssetTag,omitempty"`
}

type VirtualMachinesResourcesComputeSharedMemoryRegionV2 struct {
	SectionName     string `json:"SectionName,omitempty"`
	StartOffset     int32  `json:"StartOffset,omitempty"`
	Length          int32  `json:"Length,omitempty"`
	AllowGuestWrite bool   `json:"AllowGuestWrite,omitempty"`
	HiddenFromGuest bool   `json:"HiddenFromGuest,omitempty"`
}

type VirtualMachinesResourcesComputeMemoryV2 struct {
	Startup                       int32                                                 `json:"Startup,omitempty"`
	Backing                       string                                                `json:"Backing,omitempty"`
	EnablePrivateCompressionStore bool                                                  `json:"EnablePrivateCompressionStore,omitempty"`
	EnableHotHint                 bool                                                  `json:"EnableHotHint,omitempty"`
	EnableColdHint                bool                                                  `json:"EnableColdHint,omitempty"`
	DirectFileMappingMB           int64                                                 `json:"DirectFileMappingMB,omitempty"`
	SharedMemoryMB                int64                                                 `json:"SharedMemoryMB,omitempty"`
	SharedMemoryAccessSids        []string                                              `json:"SharedMemoryAccessSids,omitempty"`
	EnableEpf                     bool                                                  `json:"EnableEpf,omitempty"`
	Regions                       []VirtualMachinesResourcesComputeSharedMemoryRegionV2 `json:"Regions,omitempty"`
}

type VirtualMachinesResourcesComputeProcessorV2 struct {
	Count                          int32 `json:"Count,omitempty"`
	ExposeVirtualizationExtensions bool  `json:"ExposeVirtualizationExtensions,omitempty"`
	SynchronizeQPC                 bool  `json:"SynchronizeQPC,omitempty"`
	EnableSchedulerAssist          bool  `json:"EnableSchedulerAssist,omitempty"`
}

type VirtualMachinesResourcesComputeTopologyV2 struct {
	Memory    *VirtualMachinesResourcesComputeMemoryV2    `json:"Memory,omitempty"`
	Processor *VirtualMachinesResourcesComputeProcessorV2 `json:"Processor,omitempty"`
}

type VirtualMachinesResourcesStorageAttachmentV2 struct {
	Type                         string `json:"Type,omitempty"`
	Path                         string `json:"Path,omitempty"`
	IgnoreFlushes                bool   `json:"IgnoreFlushes,omitempty"`
	CachingMode                  string `json:"CachingMode,omitempty"`
	NoWriteHardening             bool   `json:"NoWriteHardening,omitempty"`
	DisableExpansionOptimization bool   `json:"DisableExpansionOptimization,omitempty"`
	IgnoreRelativeLocator        bool   `json:"IgnoreRelativeLocator,omitempty"`
	CaptureIoAttributionContext  bool   `json:"CaptureIoAttributionContext,omitempty"`
	ReadOnly                     bool   `json:"ReadOnly,omitempty"`
}

type VirtualMachinesResourcesComPortsV2 struct {
	Port1        string `json:"Port1,omitempty"`
	Port2        string `json:"Port2,omitempty"`
	DebuggerMode bool   `json:"DebuggerMode,omitempty"`
}

type VirtualMachinesResourcesStorageScsiV2 struct {
	Attachments         map[string]VirtualMachinesResourcesStorageAttachmentV2 `json:"Attachments,omitempty"`
	ChannelInstanceGuid string                                                 `json:"ChannelInstanceGuid,omitempty"`
}

type VirtualMachinesResourcesStorageVpmemDeviceV2 struct {
	HostPath    string `json:"HostPath,omitempty"`
	ReadOnly    bool   `json:"ReadOnly,omitempty"`
	ImageFormat string `json:"ImageFormat,omitempty"`
}

type VirtualMachinesResourcesStorageVpmemControllerV2 struct {
	Devices          map[string]VirtualMachinesResourcesStorageVpmemDeviceV2 `json:"Devices,omitempty"`
	MaximumCount     int32                                                   `json:"MaximumCount,omitempty"`
	MaximumSizeBytes int32                                                   `json:"MaximumSizeBytes,omitempty"`
}

type VirtualMachinesResourcesVideoMonitorV2 struct {
	HorizontalResolution int32 `json:"HorizontalResolution,omitempty"`
	VerticalResolution   int32 `json:"VerticalResolution,omitempty"`
}

type VirtualMachinesResourcesKeyboardV2 struct {
}

type VirtualMachinesResourcesMouseV2 struct {
}

type VirtualMachinesResourcesRdpV2 struct {
	AccessSids []string `json:"AccessSids,omitempty"`
}

type HvSocketHvSocketServiceConfigV2 struct {
	//  SDDL string that HvSocket will check before allowing a host process to bind  to this specific service.  If not specified, defaults to the system DefaultBindSecurityDescriptor, defined in  HvSocketSystemWpConfig in V1.
	BindSecurityDescriptor string `json:"BindSecurityDescriptor,omitempty"`
	//  SDDL string that HvSocket will check before allowing a host process to connect  to this specific service.  If not specified, defaults to the system DefaultConnectSecurityDescriptor, defined in  HvSocketSystemWpConfig in V1.
	ConnectSecurityDescriptor string `json:"ConnectSecurityDescriptor,omitempty"`
	//  If true, HvSocket will process wildcard binds for this service/system combination.  Wildcard binds are secured in the registry at  SOFTWARE/Microsoft/Windows NT/CurrentVersion/Virtualization/HvSocket/WildcardDescriptors
	AllowWildcardBinds bool `json:"AllowWildcardBinds,omitempty"`
	Disabled           bool `json:"Disabled,omitempty"`
}

//  This is the HCS Schema version of the HvSocket configuration. The VMWP version is  located in Config.Devices.IC in V1.
type HvSocketHvSocketSystemConfigV2 struct {
	//  SDDL string that HvSocket will check before allowing a host process to bind  to an unlisted service for this specific container/VM (not wildcard binds).
	DefaultBindSecurityDescriptor string `json:"DefaultBindSecurityDescriptor,omitempty"`
	//  SDDL string that HvSocket will check before allowing a host process to connect  to an unlisted service in the VM/container.
	DefaultConnectSecurityDescriptor string                                     `json:"DefaultConnectSecurityDescriptor,omitempty"`
	ServiceTable                     map[string]HvSocketHvSocketServiceConfigV2 `json:"ServiceTable,omitempty"`
}

type VirtualMachinesResourcesGuestInterfaceV2 struct {
	ConnectToBridge bool                            `json:"ConnectToBridge,omitempty"`
	BridgeFlags     int                             `json:"BridgeFlags,omitempty"` // TODO JJH Hmm. This was string from swernli, but int in Rafaels example
	HvSocketConfig  *HvSocketHvSocketSystemConfigV2 `json:"HvSocketConfig,omitempty"`
}

type VirtualMachinesResourcesStorageVSmbAlternateDataStreamV2 struct {
	Name     string `json:"Name,omitempty"`     //  Name of the alternate data stream.
	Contents string `json:"Contents,omitempty"` //  Data to be written to the alternate data stream.
}

const (
	VsmbFlagNone                     = 0x00000000
	VsmbFlagReadOnly                 = 0x00000001 // read-only shares
	VsmbFlagShareRead                = 0x00000002 // convert exclusive access to shared read access
	VsmbFlagCacheIO                  = 0x00000004 // all opens will use cached I/O
	VsmbFlagNoOplocks                = 0x00000008 // disable oplock support
	VsmbFlagTakeBackupPrivilege      = 0x00000010 // Acquire the backup privilege when attempting to open
	VsmbFlagUseShareRootIdentity     = 0x00000020 // Use the identity of the share root when opening
	VsmbFlagNoDirectmap              = 0x00000040 // disable Direct Mapping
	VsmbFlagNoLocks                  = 0x00000080 // disable Byterange locks
	VsmbFlagNoDirnotify              = 0x00000100 // disable Directory CHange Notifications
	VsmbFlagTest                     = 0x00000200 // test mode
	VsmbFlagVmSharedMemory           = 0x00000400 // share is use for VM shared memory
	VsmbFlagRestrictFileAccess       = 0x00000800 // allow access only to the files specified in AllowedFiles
	VsmbFlagForceLevelIIOplocks      = 0x00001000 // disable all oplocks except Level II
	VsmbFlagReparseBaseLayer         = 0x00002000 // Allow the host to reparse this base layer
	VsmbFlagPseudoOplocks            = 0x00004000 // Enable pseudo-oplocks
	VsmbFlagNonCacheIO               = 0x00008000 // All opens will use non-cached IO
	VsmbFlagPseudoDirnotify          = 0x00010000 // Enable pseudo directory change notifications
	VsmbFlagDisableIndexing          = 0x00020000 // Content indexing disabled by the host for all files in the share
	VsmbFlagHideAlternateDataStreams = 0x00040000 // Alternate data streams hidden to the guest (open fails, streams are not enumerated, etc.)
	VsmbFlagEnableFsctlFiltering     = 0x00080000 // Only FSCTLs listed in AllowedFsctls are allowed against any files in the share
)

type VirtualMachinesResourcesStorageVSmbShareV2 struct {
	Name                           string                                                     `json:"Name,omitempty"`
	Path                           string                                                     `json:"Path,omitempty"`
	Flags                          int32                                                      `json:"Flags,omitempty"`
	AllowedFiles                   []string                                                   `json:"AllowedFiles,omitempty"`
	AllowedFsctls                  []int32                                                    `json:"AllowedFsctls,omitempty"`
	AutoCreateAlternateDataStreams []VirtualMachinesResourcesStorageVSmbAlternateDataStreamV2 `json:"AutoCreateAlternateDataStreams,omitempty"`
}

const (
	VPlan9FlagNone                 = 0x00000000
	VPlan9FlagReadOnly             = 0x00000001 // read-only shares
	VPlan9FlagUseTcpSocket         = 0x00000002 // used for test app
	VPlan9FlagLinuxMetadata        = 0x00000004 // write Linux metadata
	VPlan9FlagCaseSensitive        = 0x00000008 // create directories in case-sensitive mode
	VPlan9FlagUseShareRootIdentity = 0x00000010 // Use the identity of the share root when opening
)

type VirtualMachinesResourcesStoragePlan9ShareV2 struct {
	Name  string `json:"Name,omitempty"`
	Path  string `json:"Path,omitempty"`
	Port  int32  `json:"Port,omitempty"`
	Flags string `json:"Flags,omitempty"`
}

type VirtualMachinesResourcesNetworkNic struct {
	EndpointID string `json:",omitempty"`
	MacAddress string `json:",omitempty"`
}

// TODO - Remaining schema objects
type VirtualMachinesDevicesV2 struct {
	COMPorts *VirtualMachinesResourcesComPortsV2               `json:"COMPorts,omitempty"`
	SCSI     map[string]VirtualMachinesResourcesStorageScsiV2  `json:"SCSI,omitempty"`
	VPMem    *VirtualMachinesResourcesStorageVpmemControllerV2 `json:"VPMem,omitempty"`
	//	NIC                 map[string]SchemaVirtualMachinesResourcesNetworkNic   `json:"NIC,omitempty"`
	VideoMonitor   *VirtualMachinesResourcesVideoMonitorV2   `json:"VideoMonitor,omitempty"`
	Keyboard       *VirtualMachinesResourcesKeyboardV2       `json:"Keyboard,omitempty"`
	Mouse          *VirtualMachinesResourcesMouseV2          `json:"Mouse,omitempty"`
	GuestInterface *VirtualMachinesResourcesGuestInterfaceV2 `json:"GuestInterface,omitempty"`
	Rdp            *VirtualMachinesResourcesRdpV2            `json:"Rdp,omitempty"`
	//	GuestCrashReporting *SchemaVirtualMachinesResourcesGuestCrashReporting    `json:"GuestCrashReporting,omitempty"`
	VirtualSMBShares []VirtualMachinesResourcesStorageVSmbShareV2  `json:"VirtualSMBShares,omitempty"`
	Plan9Shares      []VirtualMachinesResourcesStoragePlan9ShareV2 `json:"Plan9Shares,omitempty"`
	//	Licensing           *SchemaVirtualMachinesResourcesLicensing              `json:"Licensing,omitempty"`
	//	Battery             *SchemaVirtualMachinesResourcesBattery                `json:"Battery,omitempty"`
}

type VirtualMachineV2 struct {
	VmVersion       *schemaversion.SchemaVersion               `json:"VmVersion,omitempty"`
	StopOnReset     bool                                       `json:"StopOnReset,omitempty"`
	Chipset         *VirtualMachinesResourcesChipsetV2         `json:"Chipset,omitempty"`
	ComputeTopology *VirtualMachinesResourcesComputeTopologyV2 `json:"ComputeTopology,omitempty"`
	Devices         *VirtualMachinesDevicesV2                  `json:"Devices,omitempty"`
	//RestoreState *SchemaVirtualMachinesRestoreState `json:"RestoreState,omitempty"`
	//RegistryChanges *SchemaRegistryRegistryChanges `json:"RegistryChanges,omitempty"`
	//RunInSilo *SchemaVirtualMachinesSiloSettings `json:"RunInSilo,omitempty"`
	//DebugOptions *SchemaVirtualMachinesDebugOptions `json:"DebugOptions,omitempty"`
}

type ComputeSystemV2 struct {
	Owner                             string                       `json:"Owner,omitempty"`
	SchemaVersion                     *schemaversion.SchemaVersion `json:"SchemaVersion,omitempty"`
	HostingSystemId                   string                       `json:"HostingSystemId,omitempty"`
	HostedSystem                      interface{}                  `json:"HostedSystem,omitempty"`
	Container                         *ContainerV2                 `json:"Container,omitempty"`
	ShouldTerminateOnLastHandleClosed bool                         `json:"ShouldTerminateOnLastHandleClosed,omitempty"`
	VirtualMachine                    *VirtualMachineV2            `json:"VirtualMachine,omitempty"`
}

type ResourceType string
type RequestType string

type ModifySettingsRequestV2 struct {
	ResourceUri    string       `json:"ResourceUri,omitempty"`
	ResourceType   ResourceType `json:"ResourceType,omitempty"`
	RequestType    RequestType  `json:"RequestType,omitempty"`
	Settings       interface{}  `json:"Settings,omitempty"`
	HostedSettings interface{}  `json:"HostedSettings,omitempty"`
}

// ResourceType const
const (
	ResourceTypeMemory             ResourceType = "Memory"
	ResourceTypeCpuGroup           ResourceType = "CpuGroup"
	ResourceTypeMappedDirectory    ResourceType = "MappedDirectory"
	ResourceTypeMappedPipe         ResourceType = "MappedPipe"
	ResourceTypeMappedVirtualDisk  ResourceType = "MappedVirtualDisk"
	ResourceTypeNetwork            ResourceType = "Network"
	ResourceTypeVSmbShare          ResourceType = "VSmbShare"
	ResourceTypePlan9Share         ResourceType = "Plan9Share"
	ResourceTypeCombinedLayers     ResourceType = "CombinedLayers"
	ResourceTypeHvSocket           ResourceType = "HvSocket"
	ResourceTypeSharedMemoryRegion ResourceType = "SharedMemoryRegion"
	ResourceTypeVPMemDevice        ResourceType = "VPMemDevice"
	ResourceTypeGpu                ResourceType = "Gpu"
	ResourceTypeCosIndex           ResourceType = "CosIndex" // v2.1
	ResourceTypeRmid               ResourceType = "Rmid"     // v2.1
)

// RequestType const
const (
	RequestTypeAdd     RequestType  = "Add"
	RequestTypeRemove  RequestType  = "Remove"
	RequestTypeNetwork ResourceType = "Network"
	RequestTypeUpdate  ResourceType = "Update"
)

// This class is used by a modify request to add or remove a combined layers
// structure in the guest. For windows, the GCS applies a filter in ContainerRootPath
// using the specified layers as the parent content. Ignores property ScratchPath
// since the container path is already the scratch path. For linux, the GCS unions
// the specified layers and ScratchPath together, placing the resulting union
// filesystem at ContainerRootPath.
type CombinedLayersV2 struct {
	ContainerRootPath string                       `json:"ContainerRootPath,omitempty"`
	Layers            []ContainersResourcesLayerV2 `json:"Layers,omitempty"`
	ScratchPath       string                       `json:"ScratchPath,omitempty"`
}

type ProcessConfig struct {
	SchemaVersion     *schemaversion.SchemaVersion
	ApplicationName   string            `json:",omitempty"`
	CommandLine       string            `json:",omitempty"`
	CommandArgs       []string          `json:",omitempty"` // Used by Linux Containers on Windows
	User              string            `json:",omitempty"`
	WorkingDirectory  string            `json:",omitempty"`
	Environment       map[string]string `json:",omitempty"`
	EmulateConsole    bool              `json:",omitempty"`
	CreateStdInPipe   bool              `json:",omitempty"`
	CreateStdOutPipe  bool              `json:",omitempty"`
	CreateStdErrPipe  bool              `json:",omitempty"`
	ConsoleSize       [2]uint           `json:",omitempty"`
	CreateInUtilityVm bool              `json:",omitempty"` // Used by Linux Containers on Windows
	OCIProcess        interface{}       `json:"OciProcess,omitempty"`
}
