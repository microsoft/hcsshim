package bridge

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/Microsoft/go-winio/pkg/guid"
	octrace "go.opencensus.io/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
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
	OpenCensusSpanContext *trace.SpanContext `json:"ocsc,omitempty"`
}

var _ RequestMessage = &RequestBase{}

func (req *RequestBase) Base() *RequestBase {
	return req
}

func NewRequestBase(ctx context.Context, cid string) {
	r := RequestBase{
		ContainerID: cid,
	}
	if span := octrace.FromContext(ctx); span != nil {
		sc := span.SpanContext()
		scc := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID: trace.TraceID(sc.TraceID),
			SpanID:  trace.SpanID(sc.SpanID),
		})

		r.OpenCensusSpanContext = &scc

		if sc.Tracestate != nil {
			entries := sc.Tracestate.Entries()
			if len(entries) > 0 {
				if bytes, err := json.Marshal(sc.Tracestate.Entries()); err == nil {
					r.OpenCensusSpanContext.Tracestate = base64.StdEncoding.EncodeToString(bytes)
				} else {
					log.G(ctx).WithError(err).Warn("failed to encode OpenCensus Tracestate")
				}
			}
		}
	}
	return r
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
	Result     int32  // HResult
	ResultInfo Any    `json:",omitempty"`
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
