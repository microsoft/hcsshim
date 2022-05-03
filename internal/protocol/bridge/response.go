package bridge

import (
	"encoding/json"

	"github.com/Microsoft/go-winio/pkg/guid"

	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/errdefs"
)

type ResponseMessage interface {
	Base() *ResponseBase
}

type ResponseBase struct {
	Result       errdefs.HResult
	ErrorMessage string        `json:",omitempty"`
	ActivityID   guid.GUID     `json:"ActivityId,omitempty"`
	ErrorRecords []ErrorRecord `json:",omitempty"`
}

var _ ResponseMessage = &ResponseBase{}

func (resp *ResponseBase) Base() *ResponseBase {
	return resp
}

type ErrorRecord struct {
	Result       errdefs.HResult
	Message      string
	StackTrace   string `json:",omitempty"`
	ModuleName   string
	FileName     string
	Line         uint32
	FunctionName string `json:",omitempty"`
}

type NegotiateProtocolResponse struct {
	ResponseBase
	Version      uint32          `json:",omitempty"`
	Capabilities GCSCapabilities `json:",omitempty"`
}

type DumpStacksResponse struct {
	ResponseBase
	GuestStacks string
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

type ContainerGetPropertiesResponse struct {
	ResponseBase
	Properties ContainerProperties
}

type ContainerProperties schema1.ContainerProperties

func (p *ContainerProperties) MarshalText() ([]byte, error) {
	return json.Marshal((*schema1.ContainerProperties)(p))
}

func (p *ContainerProperties) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, (*schema1.ContainerProperties)(p))
}

type ContainerGetPropertiesResponseV2 struct {
	ResponseBase
	Properties ContainerPropertiesV2
}

type ContainerPropertiesV2 hcsschema.Properties

func (p *ContainerPropertiesV2) MarshalText() ([]byte, error) {
	return json.Marshal((*hcsschema.Properties)(p))
}

func (p *ContainerPropertiesV2) UnmarshalText(b []byte) error {
	return json.Unmarshal(b, (*hcsschema.Properties)(p))
}
