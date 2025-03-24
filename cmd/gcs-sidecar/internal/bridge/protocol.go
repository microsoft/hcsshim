//go:build windows
// +build windows

package bridge

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Microsoft/go-winio/pkg/guid"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

const (
	hdrSize    = 16
	hdrOffType = 0
	hdrOffSize = 4
	hdrOffID   = 8

	// maxMsgSize is the maximum size of an incoming message. This is not
	// enforced by the guest today but some maximum must be set to avoid
	// unbounded allocations.
	maxMsgSize = 0x10000

	msgTypeRequest  msgType = 0x10100000
	msgTypeResponse msgType = 0x20100000
	msgTypeNotify   msgType = 0x30100000
	msgTypeMask     msgType = 0xfff00000

	notifyContainer = 1<<8 | 1
)

// SequenceID is used to correlate requests and responses.
type SequenceID uint64

// MessageHeader is the common header present in all communications messages.
type MessageHeader struct {
	Type msgType
	Size uint32
	ID   SequenceID
}

type msgType uint32

func (typ msgType) String() string {
	var s string
	switch typ & msgTypeMask {
	case msgTypeRequest:
		s = "Request("
	case msgTypeResponse:
		s = "Response("
	case msgTypeNotify:
		s = "Notify("
		switch typ - msgTypeNotify {
		case notifyContainer:
			s += "Container"
		default:
			s += fmt.Sprintf("%#x", uint32(typ))
		}
		return s + ")"
	default:
		return fmt.Sprintf("%#x", uint32(typ))
	}
	s += rpcProc(typ &^ msgTypeMask).String()
	return s + ")"
}

type rpcProc uint32

const (
	rpcCreate rpcProc = (iota+1)<<8 | 1
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
)

func (rpc rpcProc) String() string {
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
		return "0x" + strconv.FormatUint(uint64(rpc), 16)
	}
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

type responseBase struct {
	Result       int32         // HResult
	ErrorMessage string        `json:",omitempty"`
	ActivityID   guid.GUID     `json:"ActivityId,omitempty"`
	ErrorRecords []errorRecord `json:",omitempty"`
}

func (req *requestBase) Base() *requestBase {
	return req
}

func (resp *responseBase) Base() *responseBase {
	return resp
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

type negotiateProtocolRequest struct {
	requestBase
	MinimumVersion uint32
	MaximumVersion uint32
}

type hcsschemaVersion struct {
	Major int32 `json:"Major,omitempty"`

	Minor int32 `json:"Minor,omitempty"`
}

type gcsCapabilities struct {
	SendHostCreateMessage      bool
	SendHostStartMessage       bool
	HvSocketConfigOnStartup    bool
	SendLifecycleNotifications bool
	SupportedSchemaVersions    []hcsschemaVersion
	RuntimeOsType              string
	GuestDefinedCapabilities   interface{}
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

type containerModifySettings struct {
	requestBase
	Request interface{}
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
