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

const (
	HdrSize    = 16
	HdrOffType = 0
	HdrOffSize = 4
	HdrOffID   = 8

	// maxMsgSize is the maximum size of an incoming message. This is not
	// enforced by the guest today but some maximum must be set to avoid
	// unbounded allocations.
	MaxMsgSize = 0x10000

	// LinuxGcsVsockPort is the vsock port number that the Linux GCS will
	// connect to.
	LinuxGcsVsockPort = 0x40000000
)

// e0e16197-dd56-4a10-9195-5ee7a155a838
var HvGUIDLoopback = guid.GUID{
	Data1: 0xe0e16197,
	Data2: 0xdd56,
	Data3: 0x4a10,
	Data4: [8]uint8{0x91, 0x95, 0x5e, 0xe7, 0xa1, 0x55, 0xa8, 0x38},
}

// a42e7cda-d03f-480c-9cc2-a4de20abb878
var HvGUIDParent = guid.GUID{
	Data1: 0xa42e7cda,
	Data2: 0xd03f,
	Data3: 0x480c,
	Data4: [8]uint8{0x9c, 0xc2, 0xa4, 0xde, 0x20, 0xab, 0xb8, 0x78},
}

// WindowsGcsHvsockServiceID is the hvsock service ID that the Windows GCS
// will connect to.
var WindowsGcsHvsockServiceID = guid.GUID{
	Data1: 0xacef5661,
	Data2: 0x84a1,
	Data3: 0x4e44,
	Data4: [8]uint8{0x85, 0x6b, 0x62, 0x45, 0xe6, 0x9f, 0x46, 0x20},
}

// WindowsSidecarGcsHvsockServiceID is the hvsock service ID that the Windows GCS
// sidecar will connect to. This is only used in the confidential mode.
var WindowsSidecarGcsHvsockServiceID = guid.GUID{
	Data1: 0xae8da506,
	Data2: 0xa019,
	Data3: 0x4553,
	Data4: [8]uint8{0xa5, 0x2b, 0x90, 0x2b, 0xc0, 0xfa, 0x04, 0x11},
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

type RPCProc uint32

const (
	RPCCreate RPCProc = (iota+1)<<8 | 1
	RPCStart
	RPCShutdownGraceful
	RPCShutdownForced
	RPCExecuteProcess
	RPCWaitForProcess
	RPCSignalProcess
	RPCResizeConsole
	RPCGetProperties
	RPCModifySettings
	RPCNegotiateProtocol
	RPCDumpStacks
	RPCDeleteContainerState
	RPCUpdateContainer
	RPCLifecycleNotification
)

func (rpc RPCProc) String() string {
	switch rpc {
	case RPCCreate:
		return "Create"
	case RPCStart:
		return "Start"
	case RPCShutdownGraceful:
		return "ShutdownGraceful"
	case RPCShutdownForced:
		return "ShutdownForced"
	case RPCExecuteProcess:
		return "ExecuteProcess"
	case RPCWaitForProcess:
		return "WaitForProcess"
	case RPCSignalProcess:
		return "SignalProcess"
	case RPCResizeConsole:
		return "ResizeConsole"
	case RPCGetProperties:
		return "GetProperties"
	case RPCModifySettings:
		return "ModifySettings"
	case RPCNegotiateProtocol:
		return "NegotiateProtocol"
	case RPCDumpStacks:
		return "DumpStacks"
	case RPCDeleteContainerState:
		return "DeleteContainerState"
	case RPCUpdateContainer:
		return "UpdateContainer"
	case RPCLifecycleNotification:
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
	s += RPCProc(typ &^ MsgTypeMask).String()
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
