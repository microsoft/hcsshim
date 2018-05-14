// Package prot defines any structures used in the communication between the HCS
// and the GCS. Some of these structures are also used outside the bridge as
// good ways of packaging parameters to core calls.
package prot

import (
	"encoding/json"

	"github.com/Microsoft/opengcs/service/libs/commonutils"
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

// MessageType is the type of the message.
type MessageType uint32

const (
	// MtNone is the default MessageType.
	MtNone = 0
	// MtRequest is the MessageType when a request is recieved.
	MtRequest = 0x10000000
	// MtResponse is the MessageType used to send a response.
	MtResponse = 0x20000000
	// MtNotification is the MessageType used to send a notification not
	// initiated by a request.
	MtNotification = 0x30000000
)

// MessageCategory allows splitting the identifier namespace to easily route
// similar messages for common processing.
type MessageCategory uint32

const (
	// McNone is the default category.
	McNone = 0
	// McComputeSystem is the category to define message types for compute
	// systems.
	McComputeSystem = 0x00100000
)

// GetResponseIdentifier returns the response version of the given request
// identifier. So, for example, an input of ComputeSystemCreateV1 would result
// in an output of ComputeSystemResponseCreateV1.
func GetResponseIdentifier(identifier MessageIdentifier) MessageIdentifier {
	return MessageIdentifier(MtResponse | (uint32(identifier) & ^uint32(messageTypeMask)))
}

// MessageIdentifier describes the Type field of a MessageHeader struct.
type MessageIdentifier uint32

const (
	// MiNone is the unknown identifier.
	MiNone = 0

	// ComputeSystemCreateV1 is the create container request.
	ComputeSystemCreateV1 = 0x10100101
	// ComputeSystemStartV1 is the start container request.
	ComputeSystemStartV1 = 0x10100201
	// ComputeSystemShutdownGracefulV1 is the graceful shutdown container
	// request.
	ComputeSystemShutdownGracefulV1 = 0x10100301
	// ComputeSystemShutdownForcedV1 is the forceful shutdown container request.
	ComputeSystemShutdownForcedV1 = 0x10100401
	// ComputeSystemExecuteProcessV1 is the execute process request.
	ComputeSystemExecuteProcessV1 = 0x10100501
	// ComputeSystemWaitForProcessV1 is the wait for process exit request.
	ComputeSystemWaitForProcessV1 = 0x10100601
	// ComputeSystemSignalProcessV1 is the signal process request.
	ComputeSystemSignalProcessV1 = 0x10100701
	// ComputeSystemResizeConsoleV1 is the resize console tty request.
	ComputeSystemResizeConsoleV1 = 0x10100801
	// ComputeSystemGetPropertiesV1 is the list process properties request.
	ComputeSystemGetPropertiesV1 = 0x10100901
	// ComputeSystemModifySettingsV1 is the modify container request.
	ComputeSystemModifySettingsV1 = 0x10100a01
	// ComputeSystemNegotiateProtocolV1 is the protocol negotiation request.
	ComputeSystemNegotiateProtocolV1 = 0x10100b01

	// ComputeSystemResponseCreateV1 is the create container response.
	ComputeSystemResponseCreateV1 = 0x20100101
	// ComputeSystemResponseStartV1 is the start container response.
	ComputeSystemResponseStartV1 = 0x20100201
	// ComputeSystemResponseShutdownGracefulV1 is the graceful shutdown
	// container response.
	ComputeSystemResponseShutdownGracefulV1 = 0x20100301
	// ComputeSystemResponseShutdownForcedV1 is the forceful shutdown container
	// response.
	ComputeSystemResponseShutdownForcedV1 = 0x20100401
	// ComputeSystemResponseExecuteProcessV1 is the execute process response.
	ComputeSystemResponseExecuteProcessV1 = 0x20100501
	// ComputeSystemResponseWaitForProcessV1 is the wait for process exit
	// response.
	ComputeSystemResponseWaitForProcessV1 = 0x20100601
	// ComputeSystemResponseSignalProcessV1 is the signal process response.
	ComputeSystemResponseSignalProcessV1 = 0x20100701
	// ComputeSystemResponseResizeConsoleV1 is the resize console tty response.
	ComputeSystemResponseResizeConsoleV1 = 0x20100801
	// ComputeSystemResponseGetPropertiesV1 is the list process properties
	// response.
	ComputeSystemResponseGetPropertiesV1 = 0x20100901
	// ComputeSystemResponseModifySettingsV1 is the modify container response.
	ComputeSystemResponseModifySettingsV1 = 0x20100a01
	// ComputeSystemResponseNegotiateProtocolV1 is the protocol negotiation
	// response.
	ComputeSystemResponseNegotiateProtocolV1 = 0x20100b01

	// ComputeSystemNotificationV1 is the notification identifier.
	ComputeSystemNotificationV1 = 0x30100101
)

// SequenceID is used to correlate requests and responses.
type SequenceID uint64

// MessageHeader is the common header present in all communications messages.
type MessageHeader struct {
	Type MessageIdentifier
	Size uint32
	ID   SequenceID
}

// MessageHeaderSize is the size in bytes of the MessageHeader struct.
const MessageHeaderSize = 16

/////////////////////////////////////////////////////

// ProtocolVersion is a type for the seclected HCS<->GCS protocol version of
// messages
type ProtocolVersion uint32

// Protocol versions.
const (
	PvInvalid ProtocolVersion = 0
	PvV3      ProtocolVersion = 3
	PvV4      ProtocolVersion = 4
	PvMax     ProtocolVersion = PvV4
)

// ProtocolSupport specifies the protocol versions to be used for HCS-GCS
// communication.
type ProtocolSupport struct {
	MinimumVersion         string `json:",omitempty"`
	MaximumVersion         string `json:",omitempty"`
	MinimumProtocolVersion uint32
	MaximumProtocolVersion uint32
}

// GcsCapabilities specifies the abilities and scenarios supported by this GCS.
type GcsCapabilities struct {
	// True if a create message should be sent for the hosting system itself.
	SendHostCreateMessage bool `json:",omitempty"`
	// True if a start message should be sent for the hosting system itself. If
	// SendHostCreateMessage is false, a start message will not be sent either.
	SendHostStartMessage bool `json:",omitempty"`
	// True if an HVSocket ModifySettings request should be sent immediately
	// after the create/start messages are sent (if they're sent at all). This
	// ModifySettings request would be to configure the local and parent
	// Hyper-V socket addresses of the VM, and would have a RequestType of
	// Update.
	HVSocketConfigOnStartup bool `json:"HvSocketConfigOnStartup,omitempty"`
}

// MessageBase is the base type embedded in all messages sent from the HCS to
// the GCS, as well as ContainerNotification which is sent from GCS to HCS.
type MessageBase struct {
	ContainerID string `json:"ContainerId"`
	ActivityID  string `json:"ActivityId"`
}

// NegotiateProtocol is the message from the HCS used to determine the protocol
// version that will be used for future communication.
type NegotiateProtocol struct {
	*MessageBase
	MinimumVersion uint32
	MaximumVersion uint32
}

// ContainerCreate is the message from the HCS specifying to create a container
// in the utility VM. This message won't actually create a Linux container
// inside the utility VM, but will set up the infrustructure needed to start one
// once the container's initial process is executed.
type ContainerCreate struct {
	*MessageBase
	ContainerConfig   string
	SupportedVersions ProtocolSupport `json:",omitempty"`
}

// NotificationType defines a type of notification to be sent back to the HCS.
type NotificationType string

const (
	// NtNone indicates nothing to be sent back to the HCS
	NtNone = NotificationType("None")
	// NtGracefulExit indicates a graceful exit notification to be sent back to
	// the HCS
	NtGracefulExit = NotificationType("GracefulExit")
	// NtForcedExit indicates a forced exit notification to be sent back to the
	// HCS
	NtForcedExit = NotificationType("ForcedExit")
	// NtUnexpectedExit indicates an unexpected exit notification to be sent
	// back to the HCS
	NtUnexpectedExit = NotificationType("UnexpectedExit")
	// NtReboot indicates a reboot notification to be sent back to the HCS
	NtReboot = NotificationType("Reboot")
	// NtConstructed indicates a constructed notification to be sent back to the
	// HCS
	NtConstructed = NotificationType("Constructed")
	// NtStarted indicates a started notification to be sent back to the HCS
	NtStarted = NotificationType("Started")
	// NtPaused indicates a paused notification to be sent back to the HCS
	NtPaused = NotificationType("Paused")
	// NtUnknown indicates an unknown notification to be sent back to the HCS
	NtUnknown = NotificationType("Unknown")
)

// ActiveOperation defines an operation to be associated with a notification
// sent back to the HCS.
type ActiveOperation string

const (
	// AoNone indicates no active operation
	AoNone = ActiveOperation("None")
	// AoConstruct indicates a construct active operation
	AoConstruct = ActiveOperation("Construct")
	// AoStart indicates a start active operation
	AoStart = ActiveOperation("Start")
	// AoPause indicates a pause active operation
	AoPause = ActiveOperation("Pause")
	// AoResume indicates a resume active operation
	AoResume = ActiveOperation("Resume")
	// AoShutdown indicates a shutdown active operation
	AoShutdown = ActiveOperation("Shutdown")
	// AoTerminate indicates a terminate active operation
	AoTerminate = ActiveOperation("Terminate")
)

// ContainerNotification is a message sent from the GCS to the HCS to indicate
// some kind of event. At the moment, it is only used for container exit
// notifications.
type ContainerNotification struct {
	*MessageBase
	Type       NotificationType
	Operation  ActiveOperation
	Result     int32
	ResultInfo string `json:",omitempty"`
}

// ExecuteProcessVsockStdioRelaySettings defines the port numbers for each
// stdio socket for a process.
type ExecuteProcessVsockStdioRelaySettings struct {
	StdIn  uint32 `json:",omitempty"`
	StdOut uint32 `json:",omitempty"`
	StdErr uint32 `json:",omitempty"`
}

// ExecuteProcessSettings defines the settings for a single process to be
// executed either inside or outside the container namespace.
type ExecuteProcessSettings struct {
	ProcessParameters       string
	VsockStdioRelaySettings ExecuteProcessVsockStdioRelaySettings
}

// ContainerExecuteProcess is the message from the HCS specifying to execute a
// process either inside or outside the container namespace.
type ContainerExecuteProcess struct {
	*MessageBase
	Settings ExecuteProcessSettings
}

// ContainerResizeConsole is the message from the HCS specifying to change the
// console size for the given process.
type ContainerResizeConsole struct {
	*MessageBase
	ProcessID uint32 `json:"ProcessId"`
	Height    uint16
	Width     uint16
}

// ContainerWaitForProcess is the message from the HCS specifying to wait until
// the given process exits. After receiving this message, the corresponding
// response should not be sent until the process has exited.
type ContainerWaitForProcess struct {
	*MessageBase
	ProcessID uint32 `json:"ProcessId"`
	// TimeoutInMs is currently ignored, since timeouts are handled on the host
	// side.
	TimeoutInMs uint32
}

// ContainerSignalProcess is the message from the HCS specifying to send a
// signal to the given process.
type ContainerSignalProcess struct {
	*MessageBase
	ProcessID uint32               `json:"ProcessId"`
	Options   SignalProcessOptions `json:",omitempty"`
}

// ContainerGetProperties is the message from the HCS requesting certain
// properties of the container, such as a list of its processes.
type ContainerGetProperties struct {
	*MessageBase
	Query string
}

// PropertyType is the type of property, such as memory or virtual disk, which
// is to be modified for the container.
type PropertyType string

const (
	// PtMemory is the property type for memory
	PtMemory = PropertyType("Memory")
	// PtCPUGroup is the property type for CPU group
	PtCPUGroup = PropertyType("CpuGroup")
	// PtStatistics is the property type for statistics
	PtStatistics = PropertyType("Statistics")
	// PtProcessList is the property type for a process list
	PtProcessList = PropertyType("ProcessList")
	// PtPendingUpdates is the property type for determining if there are
	// pending updates
	PtPendingUpdates = PropertyType("PendingUpdates")
	// PtTerminateOnLastHandleClosed is the property type for exiting when the
	// last handle is closed
	PtTerminateOnLastHandleClosed = PropertyType("TerminateOnLastHandleClosed")
	// PtMappedDirectory is the property type for mapped directories
	PtMappedDirectory = PropertyType("MappedDirectory")
	// PtSystemGUID is the property type for the system GUID
	PtSystemGUID = PropertyType("SystemGUID")
	// PtNetwork is the property type for networking
	PtNetwork = PropertyType("Network")
	// PtMappedPipe is the property type for mapped pipes
	PtMappedPipe = PropertyType("MappedPipe")
	// PtMappedVirtualDisk is the property type for mapped virtual disks
	PtMappedVirtualDisk = PropertyType("MappedVirtualDisk")
)

// RequestType is the type of operation to perform on a given property type.
type RequestType string

const (
	// RtAdd is the "Add" request type of operation
	RtAdd = RequestType("Add")
	// RtRemove is the "Remove" request type of operation
	RtRemove = RequestType("Remove")
	// RtUpdate is the "Update" request type of operation
	RtUpdate = RequestType("Update")
)

// ResourceModificationRequestResponse details a container resource which should
// be modified, how, and with what parameters.
type ResourceModificationRequestResponse struct {
	ResourceType PropertyType
	RequestType  RequestType `json:",omitempty"`
	Settings     interface{} `json:",omitempty"`
}

// ContainerModifySettings is the message from the HCS specifying how a certain
// container resource should be modified.
type ContainerModifySettings struct {
	*MessageBase
	Request ResourceModificationRequestResponse
}

// UnmarshalContainerModifySettings unmarshals the given bytes into a
// ContainerModifySettings message. This function is required because properties
// such as `Settings` can be of many types identified by the `ResourceType` and
// require dynamic unmarshalling.
func UnmarshalContainerModifySettings(b []byte) (*ContainerModifySettings, error) {
	// Unmarshal the message.
	var request ContainerModifySettings
	var rawSettings json.RawMessage
	request.Request.Settings = &rawSettings
	if err := commonutils.UnmarshalJSONWithHresult(b, &request); err != nil {
		return nil, errors.WithStack(err)
	}

	if request.Request.RequestType == "" {
		request.Request.RequestType = RtAdd
	}

	// Fill in the ResourceType-specific fields.
	switch request.Request.ResourceType {
	case PtMappedVirtualDisk:
		mvd := &MappedVirtualDisk{}
		if err := commonutils.UnmarshalJSONWithHresult(rawSettings, mvd); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal settings as MappedVirtualDisk")
		}
		request.Request.Settings = mvd
	case PtMappedDirectory:
		md := &MappedDirectory{}
		if err := commonutils.UnmarshalJSONWithHresult(rawSettings, md); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal settings as MappedDirectory")
		}
		request.Request.Settings = md
	default:
		return nil, errors.Errorf("invalid ResourceType '%s'", request.Request.ResourceType)
	}

	return &request, nil
}

// ErrorRecord represents a single error to be reported back to the HCS. It
// allows for specifying information about the source of the error, as well as
// an error message and stack trace.
type ErrorRecord struct {
	Result       int32
	Message      string
	StackTrace   string `json:",omitempty"`
	ModuleName   string
	FileName     string
	Line         uint32
	FunctionName string `json:",omitempty"`
}

// MessageResponseBase is the base type embedded in all messages sent from the
// GCS to the HCS except for ContainerNotification.
type MessageResponseBase struct {
	Result       int32
	ActivityID   string        `json:"ActivityId"`
	ErrorRecords []ErrorRecord `json:",omitempty"`
}

// NegotiateProtocolResponse is the message to the HCS responding to a
// NegotiateProtocol message. It specifies the prefered protocol version and
// available capabilities of the GCS.
type NegotiateProtocolResponse struct {
	*MessageResponseBase
	Version      uint32
	Capabilities GcsCapabilities
}

// ContainerCreateResponse is the message to the HCS responding to a
// ContainerCreate message. It serves a protocol negotiation function as well
// for protocol versions 3 and lower, returning protocol version information to
// the HCS.
type ContainerCreateResponse struct {
	*MessageResponseBase
	SelectedVersion         string `json:",omitempty"`
	SelectedProtocolVersion uint32
}

// ContainerExecuteProcessResponse is the message to the HCS responding to a
// ContainerExecuteProcess message. It provides back the process's pid.
type ContainerExecuteProcessResponse struct {
	*MessageResponseBase
	ProcessID uint32 `json:"ProcessId"`
}

// ContainerWaitForProcessResponse is the message to the HCS responding to a
// ContainerWaitForProcess message. It is only sent when the process has exited.
type ContainerWaitForProcessResponse struct {
	*MessageResponseBase
	ExitCode uint32
}

// ContainerGetPropertiesResponse is the message to the HCS responding to a
// ContainerGetProperties message. It contains a string representing the
// properties requested.
type ContainerGetPropertiesResponse struct {
	*MessageResponseBase
	Properties string
}

/* types added on to the current official protocol types */

// Layer represents a filesystem layer for a container.
type Layer struct {
	// Path is in this case the identifier (such as the SCSI number) of the
	// layer device.
	Path string
}

// NetworkAdapter represents a network interface and its associated
// configuration.
type NetworkAdapter struct {
	AdapterInstanceID  string `json:"AdapterInstanceId"`
	FirewallEnabled    bool
	NatEnabled         bool
	MacAddress         string `json:",omitempty"`
	AllocatedIPAddress string `json:"AllocatedIpAddress,omitempty"`
	HostIPAddress      string `json:"HostIpAddress,omitempty"`
	HostIPPrefixLength uint8  `json:"HostIpPrefixLength,omitempty"`
	HostDNSServerList  string `json:"HostDnsServerList,omitempty"`
	HostDNSSuffix      string `json:"HostDnsSuffix,omitempty"`
	EnableLowMetric    bool   `json:",omitempty"`
	EncapOverhead      uint16 `json:",omitempty"`
}

// MappedVirtualDisk represents a disk on the host which is mapped into a
// directory in the guest.
type MappedVirtualDisk struct {
	ContainerPath     string
	Lun               uint8 `json:",omitempty"`
	CreateInUtilityVM bool  `json:",omitempty"`
	ReadOnly          bool  `json:",omitempty"`
	AttachOnly        bool  `json:",omitempty"`
}

// MappedDirectory represents a directory on the host which is mapped to a
// directory on the guest through a technology such as Plan9.
type MappedDirectory struct {
	ContainerPath     string
	CreateInUtilityVM bool   `json:",omitempty"`
	ReadOnly          bool   `json:",omitempty"`
	Port              uint32 `json:",omitempty"`
}

// VMHostedContainerSettings is the set of settings used to specify the initial
// configuration of a container.
type VMHostedContainerSettings struct {
	Layers []Layer
	// SandboxDataPath is in this case the identifier (such as the SCSI number)
	// of the sandbox device.
	SandboxDataPath    string
	MappedVirtualDisks []MappedVirtualDisk
	MappedDirectories  []MappedDirectory
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
	// CommandLine is a space separated list of command line parameters. For
	// example, the command which sleeps for 100 seconds would be represented by
	// the CommandLine string "sleep 100".
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

// SignalProcessOptions represents the options for signaling a process.
type SignalProcessOptions struct {
	Signal int32
}

// ProcessDetails represents information about a given process.
type ProcessDetails struct {
	ProcessID uint32 `json:"ProcessId"`
}

// PropertyQuery is a query to specify which properties are requested.
type PropertyQuery struct {
	PropertyTypes []PropertyType `json:",omitempty"`
}

// Properties represents the properties of a compute system.
type Properties struct {
	ProcessList []ProcessDetails `json:",omitempty"`
}
