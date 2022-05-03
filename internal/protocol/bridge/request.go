package bridge

import (
	"context"
	"encoding/json"

	"github.com/Microsoft/go-winio/pkg/guid"
	otel "go.opentelemetry.io/otel/trace"

	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/errdefs"
	"github.com/Microsoft/hcsshim/internal/trace"
)

type RequestMessage interface {
	Base() *RequestBase
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
	OpenCensusSpanContext *otel.SpanContext `json:"ocsc,omitempty"`
}

var _ RequestMessage = &RequestBase{}

func (req *RequestBase) Base() *RequestBase {
	return req
}

func NewRequestBase(ctx context.Context, cid string) RequestBase {
	if cid == "" {
		cid = NullContainerID
	}

	return RequestBase{
		ContainerID:           cid,
		OpenCensusSpanContext: trace.GetSpanContext(ctx),
	}
}

type NegotiateProtocolRequest struct {
	RequestBase
	MinimumVersion uint32
	MaximumVersion uint32
}

type GCSCapabilities struct {
	SendHostCreateMessage      bool
	SendHostStartMessage       bool
	HvSocketConfigOnStartup    bool
	SendLifecycleNotifications bool
	SupportedSchemaVersions    []hcsschema.Version
	RuntimeOsType              string
	GuestDefinedCapabilities   interface{} //todo, see if this can be replaced with `Any`
}

type DumpStacksRequest struct {
	RequestBase
}

type DeleteContainerStateRequest struct {
	RequestBase
}

type ContainerCreate struct {
	RequestBase
	ContainerConfig Any
}

type UVMConfig struct {
	SystemType          string // must be "Container"
	TimeZoneInformation *hcsschema.TimeZoneInformation
}

type ContainerNotification struct {
	RequestBase
	Type       string // Compute.System.NotificationType
	Operation  string // Compute.System.ActiveOperation
	Result     errdefs.HResult
	ResultInfo Any `json:",omitempty"`
}

type ContainerExecuteProcess struct {
	RequestBase
	Settings ExecuteProcessSettings
}

type ExecuteProcessSettings struct {
	ProcessParameters       Any
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
	Options   interface{} `json:",omitempty"` //todo, see if this can be replaced with `Any`
}

type ContainerGetProperties struct {
	RequestBase
	Query ContainerPropertiesQuery
}

type ContainerPropertiesQuery schema1.PropertyQuery

func (q *ContainerPropertiesQuery) MarshalText() ([]byte, error) {
	return json.Marshal((*schema1.PropertyQuery)(q))
}

func (q *ContainerPropertiesQuery) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, (*schema1.PropertyQuery)(q))
}

type ContainerGetPropertiesV2 struct {
	RequestBase
	Query ContainerPropertiesQueryV2
}

type ContainerPropertiesQueryV2 hcsschema.PropertyQuery

func (q *ContainerPropertiesQueryV2) MarshalText() ([]byte, error) {
	return json.Marshal((*hcsschema.PropertyQuery)(q))
}

func (q *ContainerPropertiesQueryV2) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, (*hcsschema.PropertyQuery)(q))
}

type ContainerModifySettings struct {
	RequestBase
	Request interface{} //todo, see if this can be replaced with `Any`
}
