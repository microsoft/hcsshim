 guestrequest

 (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/opencontainers/runtime-spec/specs-go"
)

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
CombinedLayers  {
	ContainerRootPath string            `json:"ContainerRootPath,omitempty"`
	Layers            []hcsschema.Layer `json:"Layers,omitempty"`
	ScratchPath       string            `json:"ScratchPath,omitempty"`
}

// Defines the schema for hosted settings passed to GCS and/or OpenGCS

// SCSI. Scratch space for remote file-system commands, or R/W layer for containers
type LCOWMappedVirtualDisk  {
	MountPath  string   `json:"MountPath,omitempty"`
	Lun        uint8    `json:"Lun,omitempty"`
	Controller uint8    `json:"Controller,omitempty"`
	ReadOnly   bool     `json:"ReadOnly,omitempty"`
	Options    []string `json:"Options,omitempty"`
}

 WCOWMappedVirtualDisk  {
	ContainerPath string `json:"ContainerPath,omitempty"`
	Lun           int32  `json:"Lun,omitempty"`
}

 LCOWMappedDirectory  {
	MountPath string `json:"MountPath,omitempty"`
	Port      int32  `json:"Port,omitempty"`
	ShareName string `json:"ShareName,omitempty"` // If empty not using ANames (not currently supported)
	ReadOnly  bool   `json:"ReadOnly,omitempty"`
}

// Read-only layers over VPMem
 LCOWMappedVPMemDevice  {
	DeviceNumber uint32 `json:"DeviceNumber,omitempty"`
	MountPath    string `json:"MountPath,omitempty"`
}

 LCOWMappedVPCIDevice  {
	VMBusGUID string `json:"VMBusGUID,omitempty"`
}

 LCOWNetworkAdapter  {
	NamespaceID     string `json:",omitempty"`
	ID              string `json:",omitempty"`
	MacAddress      string `json:",omitempty"`
	IPAddress       string `json:",omitempty"`
	PrefixLength    uint8  `json:",omitempty"`
	GatewayAddress  string `json:",omitempty"`
	DNSSuffix       string `json:",omitempty"`
	DNSServerList   string `json:",omitempty"`
	EnableLowMetric bool   `json:",omitempty"`
	EncapOverhead   uint16 `json:",omitempty"`
}


 LCOWContainerConstraints {
	Windows specs.WindowsResources `json:",omitempty"`
	Linux   specs.LinuxResources   `json:",omitempty"`
}

 ResourceType

 (
	// These are constants for v2 schema modify guest requests.
	ResourceTypeMappedDirectory      ResourceType = "MappedDirectory"
	ResourceTypeMappedVirtualDisk    ResourceType = "MappedVirtualDisk"
	ResourceTypeNetwork              ResourceType = "Network"
	ResourceTypeNetworkNamespace     ResourceType = "NetworkNamespace"
	ResourceTypeCombinedLayers       ResourceType = "CombinedLayers"
	ResourceTypeVPMemDevice          ResourceType = "VPMemDevice"
	ResourceTypeVPCIDevice           ResourceType = "VPCIDevice"
	ResourceTypeContainerConstraints ResourceType = "ContainerConstraints"
	ResourceTypeHvSocket             ResourceType = "HvSocket"
)

// GuestRequest is for modify commands passed to the guest.
 GuestRequest struct {
	RequestType  string       `json:"RequestType,omitempty"`
	ResourceType ResourceType `json:"ResourceType,omitempty"`
	Settings     interface{}  `json:"Settings,omitempty"`
}

 NetworkModifyRequest {
	AdapterId   string      `json:"AdapterId,omitempty"`
	RequestType string      `json:"RequestType,omitempty"`
	Settings    interface{} `json:"Settings,omitempty"`
}

 RS4NetworkModifyRequest  {
	AdapterInstanceId string      `json:"AdapterInstanceId,omitempty"`
	RequestType       string      `json:"RequestType,omitempty"`
	Settings          interface{} `json:"Settings,omitempty"`
}

// SignalProcessOptionsLCOW is the options passed to LCOW to signal a given
// process.
 SignalProcessOptionsLCOW  {
	Signal int `json:",omitempty"`
}

 SignalValueWCOW 

(
	SignalValueWCOWCtrlC        SignalValueWCOW  "CtrlC"
	SignalValueWCOWCtrlBreak    SignalValueWCOW  "CtrlBreak"
	SignalValueWCOWCtrlClose    SignalValueWCOW  "CtrlClose"
	SignalValueWCOWCtrlLogOff   SignalValueWCOW  "CtrlLogOff"
	SignalValueWCOWCtrlShutdown SignalValueWCOW  "CtrlShutdown"
)

// SignalProcessOptionsWCOW is the options passed to WCOW to signal a given
// process.
 SignalProcessOptionsWCOW  {
	Signal SignalValueWCOW `json:",omitempty"`
}
