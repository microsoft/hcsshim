// This package contains errors commonly encountered when interacting with the Windows HCS APIs,
// Win32 APIs in general, and frequently used throughout the hcsshim repo.
package errdefs

import (
	"errors"
	"net"
	"syscall"
)

// todo: verify that `Errno` definitions are OS-agnostic and can be used by Linux GCS
// [syscall.Errno] should be uintptr for Linux (Unix) and Windows

const (
	// ErrAccessIsDenied is an error when access is denied
	ErrAccessDenied = syscall.Errno(0x5)

	// ErrVmcomputeOperationAccessIsDenied is an error which can be encountered when enumerating compute systems in RS1/RS2
	// builds when the underlying silo might be in the process of terminating. HCS was fixed in RS3.
	ErrVmcomputeOperationAccessIsDenied = ErrAccessDenied

	// ErrInvalidHandle is an error that can be encountered when querying the properties of a compute system when the handle to that
	// compute system has already been closed.
	ErrInvalidHandle = syscall.Errno(0x6)

	// ErrInvalidData is an error encountered when the request being sent to HCS is invalid or unsupported
	// decimal -2147024883 / hex 0x8007000d
	ErrInvalidData = syscall.Errno(0xd)

	// ErrElementNotFound is an error encountered when the object being referenced does not exist
	ErrNotSupported = syscall.Errno(0x32)

	// ErrProcNotFound is an error encountered when a procedure look up fails.
	ErrProcNotFound = syscall.Errno(0x7f)

	// ErrElementNotFound is an error encountered when the object being referenced does not exist
	ErrElementNotFound = syscall.Errno(0x490)

	// ErrProcessAlreadyStopped is returned by HCS if the process we're trying to kill has already been stopped.
	ErrProcessAlreadyStopped = syscall.Errno(0x8037011f)

	// ErrVmcomputeOperationPending is an error encountered when the operation is being completed asynchronously
	ErrVmcomputeOperationPending = syscall.Errno(0xc0370103)

	// ErrVmcomputeOperationInvalidState is an error encountered when the compute system is not in a valid state for the requested operation
	ErrVmcomputeOperationInvalidState = syscall.Errno(0xc0370105)

	// ErrVmcomputeUnexpectedExit is an error encountered when the compute system terminates unexpectedly
	ErrVmcomputeUnexpectedExit = syscall.Errno(0xc0370106)

	// ErrVmcomputeUnknownMessage is an error encountered guest compute system doesn't support the message
	ErrVmcomputeUnknownMessage = syscall.Errno(0xc037010b)

	// ErrVmcomputeInvalidJSON is an error encountered when the compute system does not support/understand the messages sent by management
	ErrVmcomputeInvalidJSON = syscall.Errno(0xc037010d)

	// ErrComputeSystemDoesNotExist is an error encountered when the container being operated on no longer exists
	ErrComputeSystemDoesNotExist = syscall.Errno(0xc037010e)

	// ErrVmcomputeAlreadyStopped is an error encountered when a shutdown or terminate request is made on a stopped container
	ErrVmcomputeAlreadyStopped = syscall.Errno(0xc0370110)
)

var (
	// ErrHandleClose is an error encountered when the handle generating the notification being waited on has been closed
	ErrHandleClose = errors.New("the handle generating this notification has been closed")

	// ErrAlreadyClosed is an error encountered when using a handle that has been closed by the Close method
	ErrAlreadyClosed = errors.New("the handle has already been closed")

	// ErrInvalidNotificationType is an error encountered when an invalid notification type is used
	ErrInvalidNotificationType = errors.New("invalid notification type")

	// ErrInvalidProcessState is an error encountered when the process is not in a valid state for the requested operation
	ErrInvalidProcessState = errors.New("the process is in an invalid state for the attempted operation")

	// ErrTimeout is an error encountered when waiting on a notification times out
	ErrTimeout = errors.New("timeout waiting for notification")

	// ErrUnexpectedContainerExit is the error encountered when a container exits while waiting for
	// a different expected notification
	ErrUnexpectedContainerExit = errors.New("unexpected container exit")

	// ErrUnexpectedProcessAbort is the error encountered when communication with the compute service
	// is lost while waiting for a notification
	ErrUnexpectedProcessAbort = errors.New("lost communication with compute service")

	// ErrUnexpectedValue is an error encountered when HCS returns an invalid value
	ErrUnexpectedValue = errors.New("unexpected value returned from HCS")

	// ErrOperationDenied is an error when HCS attempts an operation that is explicitly denied
	ErrOperationDenied = errors.New("operation denied")

	// ErrNotSupported is an error encountered when HCS doesn't support the request
	ErrPlatformNotSupported = errors.New("unsupported platform request")
)

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
