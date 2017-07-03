// Package prot defines any structures used in the communication between the
// HCS and the GCS. Some of these structures are also used outside the bridge
// as good ways of packaging parameters to core calls.
package prot

import (
	"encoding/json"

	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

//////////// Code for the Message Header ////////////
// Message Identifiers as present in the message header are subdivided into
// various pieces of information.
//
// +---+----+-----+----+
// | T | CC | III | VV |
// +---+----+-----+----+
//
// T   - 4 Bits    Type
// CC  - 8 Bits    Category
// III - 12 Bits   Message Id
// VV  - 8 Bits    Version

const (
	messageTypeMask     = 0xF0000000
	messageCategoryMask = 0x0FF00000
	messageIDMask       = 0x000FFF00
	messageVersionMask  = 0x000000FF

	messageIDShift      = 8
	messageVersionShift = 0
)

// The type of the message.
type MessageType uint32

const (
	MT_None         = 0
	MT_Request      = 0x10000000
	MT_Response     = 0x20000000
	MT_Notification = 0x30000000
)

// Categories allow splitting the identifier namespace to easily route similar
// messages for common processing.
type MessageCategory uint32

const (
	MC_None          = 0
	MC_ComputeSystem = 0x00100000
)

// GetResponseIdentifier returns the response version of the given request
// identifier. So, for example, an input of ComputeSystemCreate_v1 would result
// in an output of ComputeSystemResponseCreate_v1.
func GetResponseIdentifier(identifier MessageIdentifier) MessageIdentifier {
	return MessageIdentifier(MT_Response | (uint32(identifier) & ^uint32(messageTypeMask)))
}

// The MessageIdentifiers type describes the Type field of a MessageHeader
// struct.
type MessageIdentifier uint32

const (
	MI_None = 0

	// ComputeSystem requests.
	ComputeSystemCreate_v1           = 0x10100101
	ComputeSystemStart_v1            = 0x10100201
	ComputeSystemShutdownGraceful_v1 = 0x10100301
	ComputeSystemShutdownForced_v1   = 0x10100401
	ComputeSystemExecuteProcess_v1   = 0x10100501
	ComputeSystemWaitForProcess_v1   = 0x10100601
	ComputeSystemTerminateProcess_v1 = 0x10100701
	ComputeSystemResizeConsole_v1    = 0x10100801
	ComputeSystemGetProperties_v1    = 0x10100901
	ComputeSystemModifySettings_v1   = 0x10100a01

	// ComputeSystem responses.
	ComputeSystemResponseCreate_v1           = 0x20100101
	ComputeSystemResponseStart_v1            = 0x20100201
	ComputeSystemResponseShutdownGraceful_v1 = 0x20100301
	ComputeSystemResponseShutdownForced_v1   = 0x20100401
	ComputeSystemResponseExecuteProcess_v1   = 0x20100501
	ComputeSystemResponseWaitForProcess_v1   = 0x20100601
	ComputeSystemResponseTerminateProcess_v1 = 0x20100701
	ComputeSystemResponseResizeConsole_v1    = 0x20100801
	ComputeSystemResponseGetProperties_v1    = 0x20100901
	ComputeSystemResponseModifySettings_v1   = 0x20100a01

	// ComputeSystem notifications.
	ComputeSystemNotification_v1 = 0x30100101
)

// SequenceIds are used to correlate requests and responses.
type SequenceId uint64

// MessageHeader is the common header present in all communications messages.
type MessageHeader struct {
	Type MessageIdentifier
	Size uint32
	Id   SequenceId
}

const MessageHeaderSize = 16

/////////////////////////////////////////////////////

// Protocol version.
const (
	PV_Invalid = 0
	PV_V1      = 1
	PV_V2      = 2
	PV_V3      = 3
)

type ProtocolSupport struct {
	MinimumVersion         string `json:",omitempty"`
	MaximumVersion         string `json:",omitempty"`
	MinimumProtocolVersion uint32
	MaximumProtocolVersion uint32
}

type MessageBase struct {
	ContainerId string
	ActivityId  string
}

type ContainerCreate struct {
	*MessageBase
	ContainerConfig   string
	SupportedVersions ProtocolSupport `json:",omitempty"`
}

type NotificationType string

const (
	NT_None           = NotificationType("None")
	NT_GracefulExit   = NotificationType("GracefulExit")
	NT_ForcedExit     = NotificationType("ForcedExit")
	NT_UnexpectedExit = NotificationType("UnexpectedExit")
	NT_Reboot         = NotificationType("Reboot")
	NT_Constructed    = NotificationType("Constructed")
	NT_Started        = NotificationType("Started")
	NT_Paused         = NotificationType("Paused")
	NT_Unknown        = NotificationType("Unknown")
)

type ActiveOperation string

const (
	AO_None      = ActiveOperation("None")
	AO_Construct = ActiveOperation("Construct")
	AO_Start     = ActiveOperation("Start")
	AO_Pause     = ActiveOperation("Pause")
	AO_Resume    = ActiveOperation("Resume")
	AO_Shutdown  = ActiveOperation("Shutdown")
	AO_Terminate = ActiveOperation("Terminate")
)

type ContainerNotification struct {
	*MessageBase
	Type       NotificationType
	Operation  ActiveOperation
	Result     int32
	ResultInfo string `json:",omitempty"`
}

type ExecuteProcessVsockStdioRelaySettings struct {
	StdIn  uint32 `json:",omitempty"`
	StdOut uint32 `json:",omitempty"`
	StdErr uint32 `json:",omitempty"`
}

type ExecuteProcessSettings struct {
	ProcessParameters       string
	VsockStdioRelaySettings ExecuteProcessVsockStdioRelaySettings
}

type ContainerExecuteProcess struct {
	*MessageBase
	Settings ExecuteProcessSettings
}

type ContainerResizeConsole struct {
	*MessageBase
	ProcessId uint32
	Height    uint16
	Width     uint16
}

type ContainerWaitForProcess struct {
	*MessageBase
	ProcessId uint32
	// TimeoutInMs is currently ignored, since timeouts are handled on the host
	// side.
	TimeoutInMs uint32
}

type ContainerTerminateProcess struct {
	*MessageBase
	ProcessId uint32
}

type ContainerGetProperties struct {
	*MessageBase
	Query string
}

type PropertyType string

const (
	PT_Memory                      = PropertyType("Memory")
	PT_CpuGroup                    = PropertyType("CpuGroup")
	PT_Statistics                  = PropertyType("Statistics")
	PT_ProcessList                 = PropertyType("ProcessList")
	PT_PendingUpdates              = PropertyType("PendingUpdates")
	PT_TerminateOnLastHandleClosed = PropertyType("TerminateOnLastHandleClosed")
	PT_MappedDirectory             = PropertyType("MappedDirectory")
	PT_SystemGUID                  = PropertyType("SystemGUID")
	PT_Network                     = PropertyType("Network")
	PT_MappedPipe                  = PropertyType("MappedPipe")
	PT_MappedVirtualDisk           = PropertyType("MappedVirtualDisk")
)

type RequestType string

const (
	RT_Add    = RequestType("Add")
	RT_Remove = RequestType("Remove")
	RT_Update = RequestType("Update")
)

// The Settings field of ResourceModificationRequestResponse could be of many
// different types. So, in order to handle this on the GCS side, the Settings
// field is a struct which "inherits" from all the types it could represent.
// That way, when it's unmarshaled, ResourceType is checked and only the
// relevant fields are filled in.
type ResourceModificationSettings struct {
	*MappedVirtualDisk
}

type ResourceModificationRequestResponse struct {
	ResourceType PropertyType
	RequestType  RequestType `json:",omitempty"`
	Settings     interface{} `json:",omitempty"`
}

type ContainerModifySettings struct {
	*MessageBase
	Request ResourceModificationRequestResponse
}

func UnmarshalContainerModifySettings(b []byte) (*ContainerModifySettings, error) {
	// Unmarshal the message.
	var request ContainerModifySettings
	var rawSettings json.RawMessage
	request.Request.Settings = &rawSettings
	if err := json.Unmarshal(b, &request); err != nil {
		return nil, errors.WithStack(err)
	}

	// Fill in default fields.
	if request.Request.ResourceType == "" {
		request.Request.ResourceType = PT_Memory
	}
	if request.Request.RequestType == "" {
		request.Request.RequestType = RT_Add
	}

	// Fill in the ResourceType-specific fields.
	settings := ResourceModificationSettings{}
	switch request.Request.ResourceType {
	case PT_MappedVirtualDisk:
		settings.MappedVirtualDisk = &MappedVirtualDisk{}
		if err := json.Unmarshal(rawSettings, settings.MappedVirtualDisk); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal settings as MappedVirtualDisk")
		}
		request.Request.Settings = settings
	default:
		return nil, errors.Errorf("invalid ResourceType %s", request.Request.ResourceType)
	}

	return &request, nil
}

type ErrorRecord struct {
	Result       int32
	Message      string
	ModuleName   string
	FileName     string
	Line         uint32
	FunctionName string `json:",omitempty"`
}

type MessageResponseBase struct {
	Result       int32
	ActivityId   string
	ErrorRecords []ErrorRecord `json:",omitempty"`
}

type ContainerCreateResponse struct {
	*MessageResponseBase
	SelectedVersion         string `json:",omitempty"`
	SelectedProtocolVersion uint32
}

type ContainerExecuteProcessResponse struct {
	*MessageResponseBase
	ProcessId uint32
}

type ContainerWaitForProcessResponse struct {
	*MessageResponseBase
	ExitCode uint32
}

type ContainerGetPropertiesResponse struct {
	*MessageResponseBase
	Properties string
}

/* types added on to the current official protocol types */
type Layer struct {
	// Path is in this case the identifier (such as the SCSI number) of the
	// layer device.
	Path string
}

type NetworkAdapter struct {
	AdapterInstanceId  string
	FirewallEnabled    bool
	NatEnabled         bool
	MacAddress         string `json:",omitempty"`
	AllocatedIpAddress string `json:",omitempty"`
	HostIpAddress      string `json:",omitempty"`
	HostIpPrefixLength uint8  `json:",omitempty"`
	HostDnsServerList  string `json:",omitempty"`
	HostDnsSuffix      string `json:",omitempty"`
	EnableLowMetric    bool   `json:",omitempty"`
}

type MappedVirtualDisk struct {
	ContainerPath     string
	Lun               uint8 `json:",omitempty"`
	CreateInUtilityVM bool  `json:",omitempty"`
	ReadOnly          bool  `json:",omitempty"`
}

type VmHostedContainerSettings struct {
	Layers []Layer
	// SandboxDataPath is in this case the identifier (such as the SCSI number)
	// of the sandbox device.
	SandboxDataPath    string
	MappedVirtualDisks []MappedVirtualDisk
	NetworkAdapters    []NetworkAdapter `json:",omitempty"`
}

// ProcessParameters represents any process which may be started in the utility
// VM. This covers three cases:
// 1.) It is an external process, i.e. a process running inside the utility VM
// but not inside any container. In this case, don't specify the
// OCISpecification field, but specify all other fields.
// 2.) It is the first process in a container. In this case, specify only the
// OCISpecification field, and not the other fields.
// 3.) It is a container process, but not the first process in that container.
// In this case, don't specify the OCISpecification field, but specify all
// other fields. This is the same as if it were an external process.
type ProcessParameters struct {
	// CommandLine is a space separated list of command line parameters.  For
	// example, the command which sleeps for 100 seconds would be represented
	// by the CommandLine string "sleep 100".
	CommandLine string `json:",omitempty"`
	// CommandArgs is a list of strings representing the command to execute. If
	// it is not empty, it will be used by the GCS. If it is empty, CommandLine
	// will be used instead.
	CommandArgs      []string          `json:",omitempty"`
	WorkingDirectory string            `json:",omitempty"`
	Environment      map[string]string `json:",omitempty"`
	EmulateConsole   bool              `json:",omitempty"`
	CreateStdInPipe  bool              `json:",omitempty"`
	CreateStdOutPipe bool              `json:",omitempty"`
	CreateStdErrPipe bool              `json:",omitempty"`
	// If IsExternal is false, the process will be created inside a container.
	// If true, it will be created external to any container. The latter is
	// useful if, for example, you want to start up a shell in the utility VM
	// for debugging/diagnostic purposes.
	IsExternal bool `json:"CreateInUtilityVM,omitempty"`
	// If this is the first process created for this container, this field must
	// be specified. Otherwise, it must be left blank and the other fields must
	// be specified.
	OCISpecification oci.Spec `json:"OciSpecification,omitempty"`
}
