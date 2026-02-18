//go:build windows

package hcs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
)

var (
	// ErrComputeSystemDoesNotExist is an error encountered when the container being operated on no longer exists
	ErrComputeSystemDoesNotExist = syscall.Errno(0xc037010e)

	// ErrElementNotFound is an error encountered when the object being referenced does not exist
	ErrElementNotFound = syscall.Errno(0x490)

	// ErrElementNotFound is an error encountered when the object being referenced does not exist
	ErrNotSupported = syscall.Errno(0x32)

	// ErrInvalidData is an error encountered when the request being sent to hcs is invalid/unsupported
	// decimal -2147024883 / hex 0x8007000d
	ErrInvalidData = syscall.Errno(0xd)

	// ErrHandleClose is an error encountered when the handle generating the notification being waited on has been closed
	ErrHandleClose = errors.New("hcsshim: the handle generating this notification has been closed")

	// ErrAlreadyClosed is an error encountered when using a handle that has been closed by the Close method
	ErrAlreadyClosed = errors.New("hcsshim: the handle has already been closed")

	// ErrInvalidNotificationType is an error encountered when an invalid notification type is used
	ErrInvalidNotificationType = errors.New("hcsshim: invalid notification type")

	// ErrInvalidProcessState is an error encountered when the process is not in a valid state for the requested operation
	ErrInvalidProcessState = errors.New("the process is in an invalid state for the attempted operation")

	// ErrTimeout is an error encountered when waiting on a notification times out
	ErrTimeout = errors.New("hcsshim: timeout waiting for notification")

	// ErrUnexpectedContainerExit is the error encountered when a container exits while waiting for
	// a different expected notification
	ErrUnexpectedContainerExit = errors.New("unexpected container exit")

	// ErrUnexpectedProcessAbort is the error encountered when communication with the compute service
	// is lost while waiting for a notification
	ErrUnexpectedProcessAbort = errors.New("lost communication with compute service")

	// ErrUnexpectedValue is an error encountered when hcs returns an invalid value
	ErrUnexpectedValue = errors.New("unexpected value returned from hcs")

	// ErrOperationDenied is an error when hcs attempts an operation that is explicitly denied
	ErrOperationDenied = errors.New("operation denied")

	// ErrVmcomputeAlreadyStopped is an error encountered when a shutdown or terminate request is made on a stopped container
	ErrVmcomputeAlreadyStopped = syscall.Errno(0xc0370110)

	// ErrVmcomputeOperationPending is an error encountered when the operation is being completed asynchronously
	ErrVmcomputeOperationPending = syscall.Errno(0xC0370103)

	// ErrVmcomputeOperationInvalidState is an error encountered when the compute system is not in a valid state for the requested operation
	ErrVmcomputeOperationInvalidState = syscall.Errno(0xc0370105)

	// ErrProcNotFound is an error encountered when a procedure look up fails.
	ErrProcNotFound = syscall.Errno(0x7f)

	// ErrVmcomputeOperationAccessIsDenied is an error which can be encountered when enumerating compute systems in RS1/RS2
	// builds when the underlying silo might be in the process of terminating. HCS was fixed in RS3.
	ErrVmcomputeOperationAccessIsDenied = syscall.Errno(0x5)

	// ErrVmcomputeInvalidJSON is an error encountered when the compute system does not support/understand the messages sent by management
	ErrVmcomputeInvalidJSON = syscall.Errno(0xc037010d)

	// ErrVmcomputeUnknownMessage is an error encountered guest compute system doesn't support the message
	ErrVmcomputeUnknownMessage = syscall.Errno(0xc037010b)

	// ErrVmcomputeUnexpectedExit is an error encountered when the compute system terminates unexpectedly
	ErrVmcomputeUnexpectedExit = syscall.Errno(0xC0370106)

	// ErrNotSupported is an error encountered when hcs doesn't support the request
	ErrPlatformNotSupported = errors.New("unsupported platform request")

	// ErrProcessAlreadyStopped is returned by hcs if the process we're trying to kill has already been stopped.
	ErrProcessAlreadyStopped = syscall.Errno(0x8037011f)

	// ErrInvalidHandle is an error that can be encountered when querying the properties of a compute system when the handle to that
	// compute system has already been closed.
	ErrInvalidHandle = syscall.Errno(0x6)
)

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
	sb := new(strings.Builder)
	ev.writeTo(sb)
	return sb.String()
}

func (ev *ErrorEvent) writeTo(b *strings.Builder) {
	// rough wag at needed length
	b.Grow(64 + len(ev.Message) + len(ev.StackTrace) + len(ev.Provider) + len(ev.Source))

	// [strings.Builder] Write* functions always return nil errors
	_, _ = b.WriteString("[Event Detail: " + ev.Message)
	if ev.StackTrace != "" {
		_, _ = b.WriteString(" Stack Trace: " + ev.StackTrace)
	}
	if ev.Provider != "" {
		_, _ = b.WriteString(" Provider: " + ev.Provider)
	}
	if ev.EventID != 0 {
		fmt.Fprintf(b, " EventID: %d", ev.EventID)
	}
	if ev.Flags != 0 {
		fmt.Fprintf(b, " flags: %d", ev.Flags)
	}
	if ev.Source != "" {
		_, _ = b.WriteString(" Source: " + ev.Source)
	}
	_ = b.WriteByte(']')
}

type hcsResult struct {
	Error        int32
	ErrorMessage string       // ErrorMessage should be the same as `windows.Errno(Err).Error()`.
	ErrorEvents  []ErrorEvent `json:"ErrorEvents,omitempty"`
	// TODO: AttributionRecords
}

func processHcsResult(ctx context.Context, resultJSON string) []ErrorEvent {
	if resultJSON == "" {
		return nil
	}

	result := &hcsResult{}
	if err := json.Unmarshal([]byte(resultJSON), result); err != nil {
		log.G(ctx).WithFields(logrus.Fields{
			logfields.JSON:  resultJSON,
			logrus.ErrorKey: err,
		}).Warn("Could not unmarshal HCS result")
		return nil
	}
	return result.ErrorEvents
}

// TODO: move [resultError] and [ErrorEvent] to schema2 and handle parsing results/data directly in vmcompute
// See: https://learn.microsoft.com/en-us/virtualization/api/hcs/schemareference#ResultError

// resultError describes an HCS operation result and allows retaining the ErrorEvents
// when an [error] is expected but an [HcsError] is not appropriate (i.e., the operation is not known).
//
// It is used in [notificationWatcher] to send the [ErrorEvent]s in a callback's notification data
// to the corresponding [waitForNotification] via a [notificationChannel].
type resultError struct {
	Err    error
	Events []ErrorEvent
}

func (e *resultError) Error() string {
	return appendErrorEvents(e.Err.Error(), e.Events)
}

func (e *resultError) Is(target error) bool {
	return errors.Is(e.Err, target)
}

func (e *resultError) Unwrap() error {
	return e.Err
}

// getEvents checks to see if err is a [resultError], and if so, returns the unwrapped error
// and [ErrorEvent]s.
//
// Currently, only [notificationWatcher] creates [resultError]s, and they are subsequently
// handled in [waitForNotification].
func getEvents(err error) ([]ErrorEvent, error) {
	var rErr *resultError
	if errors.As(err, &rErr) {
		return rErr.Events, rErr.Err
	}
	return nil, err
}

type HcsError struct {
	Op     string
	Err    error
	Events []ErrorEvent
}

var _ net.Error = &HcsError{}

func makeHCSError(op string, err error, events []ErrorEvent) error {
	// Don't double wrap errors
	var e *HcsError
	if errors.As(err, &e) {
		return err
	}

	return &HcsError{
		Op:     op,
		Err:    err,
		Events: events,
	}
}

func (e *HcsError) Error() string {
	return appendErrorEvents(e.Op+": "+e.Err.Error(), e.Events)
}

func (e *HcsError) Is(target error) bool {
	return errors.Is(e.Err, target)
}

// unwrap isnt really needed, but helpful convince function

func (e *HcsError) Unwrap() error {
	return e.Err
}

// Deprecated: net.Error.Temporary is deprecated.
func (e *HcsError) Temporary() bool {
	err := e.netError()
	return (err != nil) && err.Temporary()
}

func (e *HcsError) Timeout() bool {
	err := e.netError()
	return (err != nil) && err.Timeout()
}

func (e *HcsError) netError() (err net.Error) {
	if errors.As(e.Unwrap(), &err) {
		return err
	}
	return nil
}

// SystemError is an error encountered in HCS during an operation on a Container object
type SystemError struct {
	HcsError
	ID string
}

var _ net.Error = &SystemError{}

func (e *SystemError) Error() string {
	return appendErrorEvents(fmt.Sprintf("%s %s: %s", e.Op, e.ID, e.Err.Error()), e.Events)
}

func makeSystemError(system *System, op string, err error, events []ErrorEvent) error {
	// Don't double wrap errors
	var e *SystemError
	if errors.As(err, &e) {
		return err
	}

	return &SystemError{
		ID: system.ID(),
		HcsError: HcsError{
			Op:     op,
			Err:    err,
			Events: events,
		},
	}
}

// ProcessError is an error encountered in HCS during an operation on a Process object
type ProcessError struct {
	HcsError
	SystemID string
	Pid      int
}

var _ net.Error = &ProcessError{}

func (e *ProcessError) Error() string {
	return appendErrorEvents(fmt.Sprintf("%s %s:%d: %s", e.Op, e.SystemID, e.Pid, e.Err.Error()), e.Events)
}

func makeProcessError(process *Process, op string, err error, events []ErrorEvent) error {
	// Don't double wrap errors
	var e *ProcessError
	if errors.As(err, &e) {
		return err
	}

	return &ProcessError{
		Pid:      process.Pid(),
		SystemID: process.SystemID(),
		HcsError: HcsError{
			Op:     op,
			Err:    err,
			Events: events,
		},
	}
}

// common formatting for error strings followed by event data,
func appendErrorEvents(s string, events []ErrorEvent) string {
	if len(events) == 0 {
		return s
	}

	sb := new(strings.Builder)
	_, _ = sb.WriteString(s)
	for _, ev := range events {
		// don't join with newlines since those are ... awkward within error strings
		_, _ = sb.WriteString(": ")
		ev.writeTo(sb)
	}
	return sb.String()
}

// IsNotExist checks if an error is caused by the Container or Process not existing.
// Note: Currently, ErrElementNotFound can mean that a Process has either
// already exited, or does not exist. Both IsAlreadyStopped and IsNotExist
// will currently return true when the error is ErrElementNotFound.
func IsNotExist(err error) bool {
	return IsAny(err, ErrComputeSystemDoesNotExist, ErrElementNotFound)
}

// IsErrorInvalidHandle checks whether the error is the result of an operation carried
// out on a handle that is invalid/closed. This error popped up while trying to query
// stats on a container in the process of being stopped.
func IsErrorInvalidHandle(err error) bool {
	return errors.Is(err, ErrInvalidHandle)
}

// IsAlreadyClosed checks if an error is caused by the Container or Process having been
// already closed by a call to the Close() method.
func IsAlreadyClosed(err error) bool {
	return errors.Is(err, ErrAlreadyClosed)
}

// IsPending returns a boolean indicating whether the error is that
// the requested operation is being completed in the background.
func IsPending(err error) bool {
	return errors.Is(err, ErrVmcomputeOperationPending)
}

// IsTimeout returns a boolean indicating whether the error is caused by
// a timeout waiting for the operation to complete.
func IsTimeout(err error) bool {
	// HcsError and co. implement Timeout regardless of whether the errors they wrap do,
	// so `errors.As(err, net.Error)`` will always be true.
	// Using `errors.As(err.Unwrap(), net.Err)` wont work for general errors.
	// So first check if there an `ErrTimeout` in the chain, then convert to a net error.
	if errors.Is(err, ErrTimeout) {
		return true
	}

	var nerr net.Error
	return errors.As(err, &nerr) && nerr.Timeout()
}

// IsAlreadyStopped returns a boolean indicating whether the error is caused by
// a Container or Process being already stopped.
// Note: Currently, ErrElementNotFound can mean that a Process has either
// already exited, or does not exist. Both IsAlreadyStopped and IsNotExist
// will currently return true when the error is ErrElementNotFound.
func IsAlreadyStopped(err error) bool {
	return IsAny(err, ErrVmcomputeAlreadyStopped, ErrProcessAlreadyStopped, ErrElementNotFound)
}

// IsNotSupported returns a boolean indicating whether the error is caused by
// unsupported platform requests
// Note: Currently Unsupported platform requests can be mean either
// ErrVmcomputeInvalidJSON, ErrInvalidData, ErrNotSupported or ErrVmcomputeUnknownMessage
// is thrown from the Platform
func IsNotSupported(err error) bool {
	// If Platform doesn't recognize or support the request sent, below errors are seen
	return IsAny(err, ErrVmcomputeInvalidJSON, ErrInvalidData, ErrNotSupported, ErrVmcomputeUnknownMessage)
}

// IsOperationInvalidState returns true when err is caused by
// `ErrVmcomputeOperationInvalidState`.
func IsOperationInvalidState(err error) bool {
	return errors.Is(err, ErrVmcomputeOperationInvalidState)
}

// IsAccessIsDenied returns true when err is caused by
// `ErrVmcomputeOperationAccessIsDenied`.
func IsAccessIsDenied(err error) bool {
	return errors.Is(err, ErrVmcomputeOperationAccessIsDenied)
}

// IsAny is a vectorized version of [errors.Is], it returns true if err is one of targets.
func IsAny(err error, targets ...error) bool {
	for _, e := range targets {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
