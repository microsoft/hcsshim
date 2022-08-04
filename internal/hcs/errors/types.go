package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
)

type HcsError struct {
	Op     string
	Err    error
	Events []ErrorEvent
}

var _ net.Error = &HcsError{}

func (e *HcsError) Error() string {
	s := e.Op + ": " + e.Err.Error()
	for _, ev := range e.Events {
		s += "\n" + ev.String()
	}
	return s
}

func (e *HcsError) Is(target error) bool {
	return errors.Is(e.Err, target)
}

func (e *HcsError) Unwrap() error {
	return e.Err
}

func (e *HcsError) netError() (err net.Error) {
	if errors.As(e.Unwrap(), &err) {
		return err
	}
	return nil
}

// Deprecated: [(net.Error).Temporary()] is deprecated.
func (e *HcsError) Temporary() bool {
	err := e.netError()
	return (err != nil) && err.Temporary()
}

func (e *HcsError) Timeout() bool {
	err := e.netError()
	return (err != nil) && err.Timeout()
}

// SystemError is an error encountered in HCS during an operation on a Container object
type SystemError struct {
	HcsError
	ID string
}

var _ net.Error = &SystemError{}

func (e *SystemError) Error() string {
	s := e.Op + " " + e.ID + ": " + e.Err.Error()
	for _, ev := range e.Events {
		s += "\n" + ev.String()
	}
	return s
}

// ProcessError is an error encountered in HCS during an operation on a Process object
type ProcessError struct {
	HcsError
	SystemID string
	Pid      int
}

var _ net.Error = &ProcessError{}

func (e *ProcessError) Error() string {
	s := fmt.Sprintf("%s %s:%d: %s", e.Op, e.SystemID, e.Pid, e.Err.Error())
	for _, ev := range e.Events {
		s += "\n" + ev.String()
	}
	return s
}

type ErrorEvent struct {
	Message    string `json:"Message,omitempty"`    // Fully formated error message
	StackTrace string `json:"StackTrace,omitempty"` // Stack trace in string form
	Provider   string `json:"Provider,omitempty"`
	EventID    uint16 `json:"EventId,omitempty"`
	Flags      uint32 `json:"Flags,omitempty"`
	Source     string `json:"Source,omitempty"`
	//Data       []EventData `json:"Data,omitempty"`  // Omit this as HCS doesn't encode this well. It's more confusing to include. It is however logged in debug mode (see processHcsResult function)
}

func (ev *ErrorEvent) String() string {
	evs := "[Event Detail: " + ev.Message
	if ev.StackTrace != "" {
		evs += " Stack Trace: " + ev.StackTrace
	}
	if ev.Provider != "" {
		evs += " Provider: " + ev.Provider
	}
	if ev.EventID != 0 {
		evs = fmt.Sprintf("%s EventID: %d", evs, ev.EventID)
	}
	if ev.Flags != 0 {
		evs = fmt.Sprintf("%s flags: %d", evs, ev.Flags)
	}
	if ev.Source != "" {
		evs += " Source: " + ev.Source
	}
	evs += "]"
	return evs
}

func ErrorEventsFromHcsResult(s string) ([]ErrorEvent, error) {
	if s == "" {
		return nil, nil
	}
	result := &hcsResult{}
	if err := json.Unmarshal([]byte(s), result); err != nil {
		return nil, fmt.Errorf("could not unmarshal HCS Result %q: %w", s, err)
	}
	return result.ErrorEvents, nil
}

type hcsResult struct {
	Error        int32
	ErrorMessage string
	ErrorEvents  []ErrorEvent `json:"ErrorEvents,omitempty"`
}
