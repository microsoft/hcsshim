//go:build windows

package prot

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/bridgeutils/commonutils"
	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// LinuxGcsVsockPort is the vsock port number that the Linux GCS will
// connect to.
const LinuxGcsVsockPort = 0x40000000

// WindowsGcsHvsockServiceID is the hvsock service ID that the Windows GCS
// will connect to.
var WindowsGcsHvsockServiceID = guid.GUID{
	Data1: 0xacef5661,
	Data2: 0x84a1,
	Data3: 0x4e44,
	Data4: [8]uint8{0x85, 0x6b, 0x62, 0x45, 0xe6, 0x9f, 0x46, 0x20},
}

// WindowsGcsHvHostID is the hvsock address for the parent of the VM running the GCS
var WindowsGcsHvHostID = guid.GUID{
	Data1: 0x894cc2d6,
	Data2: 0x9d79,
	Data3: 0x424f,
	Data4: [8]uint8{0x93, 0xfe, 0x42, 0x96, 0x9a, 0xe6, 0xd8, 0xd1},
}

type AnyInString struct {
	Value interface{}
}

func (a *AnyInString) MarshalText() ([]byte, error) {
	return json.Marshal(a.Value)
}

func (a *AnyInString) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, &a.Value)
}

type RpcProc uint32

const (
	RpcCreate RpcProc = (iota+1)<<8 | 1
	RpcStart
	RpcShutdownGraceful
	RpcShutdownForced
	RpcExecuteProcess
	RpcWaitForProcess
	RpcSignalProcess
	RpcResizeConsole
	RpcGetProperties
	RpcModifySettings
	RpcNegotiateProtocol
	RpcDumpStacks
	RpcDeleteContainerState
	RpcUpdateContainer
	RpcLifecycleNotification
)

func (rpc RpcProc) String() string {
	switch rpc {
	case RpcCreate:
		return "Create"
	case RpcStart:
		return "Start"
	case RpcShutdownGraceful:
		return "ShutdownGraceful"
	case RpcShutdownForced:
		return "ShutdownForced"
	case RpcExecuteProcess:
		return "ExecuteProcess"
	case RpcWaitForProcess:
		return "WaitForProcess"
	case RpcSignalProcess:
		return "SignalProcess"
	case RpcResizeConsole:
		return "ResizeConsole"
	case RpcGetProperties:
		return "GetProperties"
	case RpcModifySettings:
		return "ModifySettings"
	case RpcNegotiateProtocol:
		return "NegotiateProtocol"
	case RpcDumpStacks:
		return "DumpStacks"
	case RpcDeleteContainerState:
		return "DeleteContainerState"
	case RpcUpdateContainer:
		return "UpdateContainer"
	case RpcLifecycleNotification:
		return "LifecycleNotification"
	default:
		return "0x" + strconv.FormatUint(uint64(rpc), 16)
	}
}

type MsgType uint32

const (
	MsgTypeRequest  MsgType = 0x10100000
	MsgTypeResponse MsgType = 0x20100000
	MsgTypeNotify   MsgType = 0x30100000
	MsgTypeMask     MsgType = 0xfff00000

	NotifyContainer = 1<<8 | 1
)

func (typ MsgType) String() string {
	var s string
	switch typ & MsgTypeMask {
	case MsgTypeRequest:
		s = "Request("
	case MsgTypeResponse:
		s = "Response("
	case MsgTypeNotify:
		s = "Notify("
		switch typ - MsgTypeNotify {
		case NotifyContainer:
			s += "Container"
		default:
			s += fmt.Sprintf("%#x", uint32(typ))
		}
		return s + ")"
	default:
		return fmt.Sprintf("%#x", uint32(typ))
	}
	s += RpcProc(typ &^ MsgTypeMask).String()
	return s + ")"
}

// Ocspancontext is the internal JSON representation of the OpenCensus
// `trace.SpanContext` for fowarding to a GCS that supports it.
type Ocspancontext struct {
	// TraceID is the `hex` encoded string of the OpenCensus
	// `SpanContext.TraceID` to propagate to the guest.
	TraceID string `json:",omitempty"`
	// SpanID is the `hex` encoded string of the OpenCensus `SpanContext.SpanID`
	// to propagate to the guest.
	SpanID string `json:",omitempty"`

	// TraceOptions is the OpenCensus `SpanContext.TraceOptions` passed through
	// to propagate to the guest.
	TraceOptions uint32 `json:",omitempty"`

	// Tracestate is the `base64` encoded string of marshaling the OpenCensus
	// `SpanContext.TraceState.Entries()` to JSON.
	//
	// If `SpanContext.Tracestate == nil ||
	// len(SpanContext.Tracestate.Entries()) == 0` this will be `""`.
	Tracestate string `json:",omitempty"`
}

type RequestBase struct {
	ContainerID string    `json:"ContainerId"`
	ActivityID  guid.GUID `json:"ActivityId"`

	// OpenCensusSpanContext is the encoded OpenCensus `trace.SpanContext` if
	// set when making the request.
	//
	// NOTE: This is not a part of the protocol but because its a JSON protocol
	// adding fields is a non-breaking change. If the guest supports it this is
	// just additive context.
	OpenCensusSpanContext *Ocspancontext `json:"ocsc,omitempty"`
}

func (req *RequestBase) Base() *RequestBase {
	return req
}

type ResponseBase struct {
	Result       int32                     // HResult
	ErrorMessage string                    `json:",omitempty"`
	ActivityID   guid.GUID                 `json:"ActivityId,omitempty"`
	ErrorRecords []commonutils.ErrorRecord `json:",omitempty"`
}

func (resp *ResponseBase) Base() *ResponseBase {
	return resp
}

type NegotiateProtocolRequest struct {
	RequestBase
	MinimumVersion uint32
	MaximumVersion uint32
}

type NegotiateProtocolResponse struct {
	ResponseBase
	Version      uint32          `json:",omitempty"`
	Capabilities GcsCapabilities `json:",omitempty"`
}

type DumpStacksRequest struct {
	RequestBase
}

type DumpStacksResponse struct {
	ResponseBase
	GuestStacks string
}

type DeleteContainerStateRequest struct {
	RequestBase
}

type ContainerCreate struct {
	RequestBase
	ContainerConfig AnyInString
}

type UvmConfig struct {
	SystemType          string // must be "Container"
	TimeZoneInformation *hcsschema.TimeZoneInformation
}

type ContainerNotification struct {
	RequestBase
	Type       string      // Compute.System.NotificationType
	Operation  string      // Compute.System.ActiveOperation
	Result     int32       // HResult
	ResultInfo AnyInString `json:",omitempty"`
}

type ContainerExecuteProcess struct {
	RequestBase
	Settings ExecuteProcessSettings
}

type ExecuteProcessSettings struct {
	ProcessParameters       AnyInString
	StdioRelaySettings      *ExecuteProcessStdioRelaySettings      `json:",omitempty"`
	VsockStdioRelaySettings *ExecuteProcessVsockStdioRelaySettings `json:",omitempty"`
}

type ExecuteProcessStdioRelaySettings struct {
	StdIn  *guid.GUID `json:",omitempty"`
	StdOut *guid.GUID `json:",omitempty"`
	StdErr *guid.GUID `json:",omitempty"`
}

type ExecuteProcessVsockStdioRelaySettings struct {
	StdIn  uint32 `json:",omitempty"`
	StdOut uint32 `json:",omitempty"`
	StdErr uint32 `json:",omitempty"`
}

type ContainerResizeConsole struct {
	RequestBase
	ProcessID uint32 `json:"ProcessId"`
	Height    uint16
	Width     uint16
}

type ContainerWaitForProcess struct {
	RequestBase
	ProcessID   uint32 `json:"ProcessId"`
	TimeoutInMs uint32
}

type ContainerSignalProcess struct {
	RequestBase
	ProcessID uint32      `json:"ProcessId"`
	Options   interface{} `json:",omitempty"`
}

type ContainerPropertiesQuery schema1.PropertyQuery

func (q *ContainerPropertiesQuery) MarshalText() ([]byte, error) {
	return json.Marshal((*schema1.PropertyQuery)(q))
}

func (q *ContainerPropertiesQuery) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, (*schema1.PropertyQuery)(q))
}

type ContainerPropertiesQueryV2 hcsschema.PropertyQuery

func (q *ContainerPropertiesQueryV2) MarshalText() ([]byte, error) {
	return json.Marshal((*hcsschema.PropertyQuery)(q))
}

func (q *ContainerPropertiesQueryV2) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, (*hcsschema.PropertyQuery)(q))
}

type ContainerGetProperties struct {
	RequestBase
	Query ContainerPropertiesQuery
}

type ContainerGetPropertiesV2 struct {
	RequestBase
	Query ContainerPropertiesQueryV2
}

type ContainerModifySettings struct {
	RequestBase
	Request interface{}
}

type GcsCapabilities struct {
	SendHostCreateMessage      bool
	SendHostStartMessage       bool
	HvSocketConfigOnStartup    bool
	SendLifecycleNotifications bool
	SupportedSchemaVersions    []hcsschema.Version
	RuntimeOsType              string
	GuestDefinedCapabilities   json.RawMessage
}

type ContainerCreateResponse struct {
	ResponseBase
}

type ContainerExecuteProcessResponse struct {
	ResponseBase
	ProcessID uint32 `json:"ProcessId"`
}

type ContainerWaitForProcessResponse struct {
	ResponseBase
	ExitCode uint32
}

type ContainerProperties schema1.ContainerProperties

func (p *ContainerProperties) MarshalText() ([]byte, error) {
	return json.Marshal((*schema1.ContainerProperties)(p))
}

func (p *ContainerProperties) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, (*schema1.ContainerProperties)(p))
}

type ContainerPropertiesV2 hcsschema.Properties

func (p *ContainerPropertiesV2) MarshalText() ([]byte, error) {
	return json.Marshal((*hcsschema.Properties)(p))
}

func (p *ContainerPropertiesV2) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, (*hcsschema.Properties)(p))
}

type ContainerGetPropertiesResponse struct {
	ResponseBase
	Properties ContainerProperties
}

type ContainerGetPropertiesResponseV2 struct {
	ResponseBase
	Properties ContainerPropertiesV2
}
