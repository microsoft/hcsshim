package guestrequest

// These are constants for v2 schema modify requests.

type RequestType string
type ResourceType string

// RequestType const
const (
	RequestTypeAdd    RequestType = "Add"
	RequestTypeRemove RequestType = "Remove"
	RequestTypePreAdd RequestType = "PreAdd" // For networking
	RequestTypeUpdate RequestType = "Update"
)

type SignalValueWCOW string

const (
	SignalValueWCOWCtrlC        SignalValueWCOW = "CtrlC"
	SignalValueWCOWCtrlBreak    SignalValueWCOW = "CtrlBreak"
	SignalValueWCOWCtrlClose    SignalValueWCOW = "CtrlClose"
	SignalValueWCOWCtrlLogOff   SignalValueWCOW = "CtrlLogOff"
	SignalValueWCOWCtrlShutdown SignalValueWCOW = "CtrlShutdown"
)

// ModificationRequest is for modify commands passed to the guest.
type ModificationRequest struct {
	RequestType  RequestType  `json:"RequestType,omitempty"`
	ResourceType ResourceType `json:"ResourceType,omitempty"`
	Settings     interface{}  `json:"Settings,omitempty"`
}

type NetworkModifyRequest struct {
	AdapterId   string      `json:"AdapterId,omitempty"` //nolint:stylecheck
	RequestType RequestType `json:"RequestType,omitempty"`
	Settings    interface{} `json:"Settings,omitempty"`
}

type RS4NetworkModifyRequest struct {
	AdapterInstanceId string      `json:"AdapterInstanceId,omitempty"` //nolint:stylecheck
	RequestType       RequestType `json:"RequestType,omitempty"`
	Settings          interface{} `json:"Settings,omitempty"`
}
