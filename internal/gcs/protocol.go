//go:build windows

package gcs

import (
	"encoding/json"
	"strconv"

	"github.com/Microsoft/go-winio/pkg/guid"
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

type anyInString struct {
	Value interface{}
}

func (a *anyInString) MarshalText() ([]byte, error) {
	return json.Marshal(a.Value)
}

func (a *anyInString) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, &a.Value)
}

// todo (helsaawy):
//  - add types for msg, msgCategory, and msgVersion
//  - remove version number from notifyContainer and rpcProcs
//  - add constructor to msg from rpc/msgType
//  - break out mstType.String() into .String for component types

/*
bridge message identifiers in message header:

+---+----+-----+----+
| T | CC | III | VV |
+---+----+-----+----+

T	4 Bits		Type
CC	8 Bits		Category
III	12 Bits		Message Id
VV	8 Bits		Version

Type:
	None			0x0
	Request			0x1
	Response		0x2
	Notify 			0x3

Category:
	None			0x00
	ComputeSystem 	0x01

Message ID:
	request, response, or notification type

Version:
	v1				0x01
*/

const (
	msgIDShift   = 8
	msgTypeShift = 28

	msgTypeMask     = 0xf0000000
	msgCategoryMask = 0x0ff00000
	msgIDMask       = 0x000fff00
	msgVersionMask  = 0x000000ff
)

type msgIdentifier uint32

func newMsgIdentifier(t msgType, id msgID) msgIdentifier {
	return msgIdentifier(t) | msgIdentifier(msgCatContainer) | msgIdentifier(id) | msgIdentifier(msgVersionV1)
}

func (h msgIdentifier) toMsgType(t msgType) msgIdentifier {
	return msgIdentifier(t) | (h &^ msgTypeMask)
}

// cannot call this `type`, since its a keyword
// so name all getters as `msg*`
func (h msgIdentifier) msgType() msgType {
	return msgType(h & msgTypeMask)
}

func (h msgIdentifier) msgCategory() msgCategory {
	return msgCategory(h & msgCategoryMask)
}

func (h msgIdentifier) msgID() msgID {
	return msgID(h & msgIDMask)
}

func (h msgIdentifier) msgVersion() msgVersion {
	return msgVersion(h & msgVersionMask)
}

func (h msgIdentifier) String() string {
	t := h.msgType()
	id := h.msgID()
	s := ""
	switch t {
	case msgTypeRequest, msgTypeResponse:
		s = id.rpcString()
	case msgTypeNotify:
		s = id.notifyString()
	default:
		s = id.String()
	}
	return t.String() + "(" + s + ")"
}

type msgType uint32

const (
	msgTypeNone msgType = iota << msgTypeShift
	msgTypeRequest
	msgTypeResponse
	msgTypeNotify
)

func (t msgType) String() string {
	switch t {
	case msgTypeRequest:
		return "Request"
	case msgTypeResponse:
		return "Response"
	case msgTypeNotify:
		return "Notify"
	default:
		return "0x" + strconv.FormatUint(uint64(t), 16)
	}
}

type msgCategory uint32

const msgCatContainer msgCategory = 0x00100000

type msgID uint32

const (
	msgIDNone msgID = iota << msgIDShift

	// for request and response message types

	rpcCreate
	rpcStart
	rpcShutdownGraceful
	rpcShutdownForced
	rpcExecuteProcess
	rpcWaitForProcess
	rpcSignalProcess
	rpcResizeConsole
	rpcGetProperties
	rpcModifySettings
	rpcNegotiateProtocol
	rpcDumpStacks
	rpcDeleteContainerState
	rpcUpdateContainer
	rpcLifecycleNotification

	// for notify message types

	notifyContainer = 1 << msgIDShift
)

func (id msgID) String() string {
	return "0x" + strconv.FormatUint(uint64(id), 16)
}

func (rpc msgID) rpcString() string {
	switch rpc {
	case rpcCreate:
		return "Create"
	case rpcStart:
		return "Start"
	case rpcShutdownGraceful:
		return "ShutdownGraceful"
	case rpcShutdownForced:
		return "ShutdownForced"
	case rpcExecuteProcess:
		return "ExecuteProcess"
	case rpcWaitForProcess:
		return "WaitForProcess"
	case rpcSignalProcess:
		return "SignalProcess"
	case rpcResizeConsole:
		return "ResizeConsole"
	case rpcGetProperties:
		return "GetProperties"
	case rpcModifySettings:
		return "ModifySettings"
	case rpcNegotiateProtocol:
		return "NegotiateProtocol"
	case rpcDumpStacks:
		return "DumpStacks"
	case rpcDeleteContainerState:
		return "DeleteContainerState"
	case rpcUpdateContainer:
		return "UpdateContainer"
	case rpcLifecycleNotification:
		return "LifecycleNotification"
	default:
		return "<unknown RPC>"
	}
}

func (n msgID) notifyString() string {
	switch n {
	case notifyContainer:
		return "Container"
	default:
		return "<unknown notification>"
	}
}

type msgVersion uint32

const msgVersionV1 msgVersion = 0x1

// ocspancontext is the internal JSON representation of the OpenCensus
// `trace.SpanContext` for fowarding to a GCS that supports it.
type ocspancontext struct {
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

type requestBase struct {
	ContainerID string    `json:"ContainerId"`
	ActivityID  guid.GUID `json:"ActivityId"`

	// OpenCensusSpanContext is the encoded OpenCensus `trace.SpanContext` if
	// set when making the request.
	//
	// NOTE: This is not a part of the protocol but because its a JSON protocol
	// adding fields is a non-breaking change. If the guest supports it this is
	// just additive context.
	OpenCensusSpanContext *ocspancontext `json:"ocsc,omitempty"`
}

func (req *requestBase) Base() *requestBase {
	return req
}

type responseBase struct {
	Result       int32         // HResult
	ErrorMessage string        `json:",omitempty"`
	ActivityID   guid.GUID     `json:"ActivityId,omitempty"`
	ErrorRecords []errorRecord `json:",omitempty"`
}

type errorRecord struct {
	Result       int32 // HResult
	Message      string
	StackTrace   string `json:",omitempty"`
	ModuleName   string
	FileName     string
	Line         uint32
	FunctionName string `json:",omitempty"`
}

func (resp *responseBase) Base() *responseBase {
	return resp
}

type negotiateProtocolRequest struct {
	requestBase
	MinimumVersion uint32
	MaximumVersion uint32
}

type negotiateProtocolResponse struct {
	responseBase
	Version      uint32          `json:",omitempty"`
	Capabilities gcsCapabilities `json:",omitempty"`
}

type dumpStacksRequest struct {
	requestBase
}

type dumpStacksResponse struct {
	responseBase
	GuestStacks string
}

type deleteContainerStateRequest struct {
	requestBase
}

type containerCreate struct {
	requestBase
	ContainerConfig anyInString
}

type uvmConfig struct {
	SystemType          string // must be "Container"
	TimeZoneInformation *hcsschema.TimeZoneInformation
}

type containerNotification struct {
	requestBase
	Type       string      // Compute.System.NotificationType
	Operation  string      // Compute.System.ActiveOperation
	Result     int32       // HResult
	ResultInfo anyInString `json:",omitempty"`
}

type containerExecuteProcess struct {
	requestBase
	Settings executeProcessSettings
}

type executeProcessSettings struct {
	ProcessParameters       anyInString
	StdioRelaySettings      *executeProcessStdioRelaySettings      `json:",omitempty"`
	VsockStdioRelaySettings *executeProcessVsockStdioRelaySettings `json:",omitempty"`
}

type executeProcessStdioRelaySettings struct {
	StdIn  *guid.GUID `json:",omitempty"`
	StdOut *guid.GUID `json:",omitempty"`
	StdErr *guid.GUID `json:",omitempty"`
}

type executeProcessVsockStdioRelaySettings struct {
	StdIn  uint32 `json:",omitempty"`
	StdOut uint32 `json:",omitempty"`
	StdErr uint32 `json:",omitempty"`
}

type containerResizeConsole struct {
	requestBase
	ProcessID uint32 `json:"ProcessId"`
	Height    uint16
	Width     uint16
}

type containerWaitForProcess struct {
	requestBase
	ProcessID   uint32 `json:"ProcessId"`
	TimeoutInMs uint32
}

type containerSignalProcess struct {
	requestBase
	ProcessID uint32      `json:"ProcessId"`
	Options   interface{} `json:",omitempty"`
}

type containerPropertiesQuery schema1.PropertyQuery

func (q *containerPropertiesQuery) MarshalText() ([]byte, error) {
	return json.Marshal((*schema1.PropertyQuery)(q))
}

func (q *containerPropertiesQuery) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, (*schema1.PropertyQuery)(q))
}

type containerPropertiesQueryV2 hcsschema.PropertyQuery

func (q *containerPropertiesQueryV2) MarshalText() ([]byte, error) {
	return json.Marshal((*hcsschema.PropertyQuery)(q))
}

func (q *containerPropertiesQueryV2) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, (*hcsschema.PropertyQuery)(q))
}

type containerGetProperties struct {
	requestBase
	Query containerPropertiesQuery
}

type containerGetPropertiesV2 struct {
	requestBase
	Query containerPropertiesQueryV2
}

type containerModifySettings struct {
	requestBase
	Request interface{}
}

type gcsCapabilities struct {
	SendHostCreateMessage      bool
	SendHostStartMessage       bool
	HvSocketConfigOnStartup    bool
	SendLifecycleNotifications bool
	SupportedSchemaVersions    []hcsschema.Version
	RuntimeOsType              string
	GuestDefinedCapabilities   interface{}
}

type containerCreateResponse struct {
	responseBase
}

type containerExecuteProcessResponse struct {
	responseBase
	ProcessID uint32 `json:"ProcessId"`
}

type containerWaitForProcessResponse struct {
	responseBase
	ExitCode uint32
}

type containerProperties schema1.ContainerProperties

func (p *containerProperties) MarshalText() ([]byte, error) {
	return json.Marshal((*schema1.ContainerProperties)(p))
}

func (p *containerProperties) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, (*schema1.ContainerProperties)(p))
}

type containerPropertiesV2 hcsschema.Properties

func (p *containerPropertiesV2) MarshalText() ([]byte, error) {
	return json.Marshal((*hcsschema.Properties)(p))
}

func (p *containerPropertiesV2) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, (*hcsschema.Properties)(p))
}

type containerGetPropertiesResponse struct {
	responseBase
	Properties containerProperties
}

type containerGetPropertiesResponseV2 struct {
	responseBase
	Properties containerPropertiesV2
}
