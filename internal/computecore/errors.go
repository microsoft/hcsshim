//go:build windows

package computecore

import "golang.org/x/sys/windows"

// HCS specific error codes.
//
// See [documentation] for more info.
//
// [documentation]: https://learn.microsoft.com/en-us/virtualization/api/hcs/reference/hcshresult
const (
	// The virtual machine or container exited unexpectedly while starting.
	HCS_E_TERMINATED_DURING_START windows.Errno = 0x80370100

	// The container operating system does not match the host operating system.
	HCS_E_IMAGE_MISMATCH windows.Errno = 0x80370101

	// The virtual machine could not be started because a required feature is not installed.
	HCS_E_HYPERV_NOT_INSTALLED windows.Errno = 0x80370102

	// The requested virtual machine or container operation is not valid in the current state.
	HCS_E_INVALID_STATE windows.Errno = 0x80370105

	// The virtual machine or container exited unexpectedly.
	HCS_E_UNEXPECTED_EXIT windows.Errno = 0x80370106

	// The virtual machine or container was forcefully exited.
	HCS_E_TERMINATED windows.Errno = 0x80370107

	// A connection could not be established with the container or virtual machine.
	HCS_E_CONNECT_FAILED windows.Errno = 0x80370108

	// The operation timed out because a response was not received from the virtual machine or container.
	HCS_E_CONNECTION_TIMEOUT windows.Errno = 0x80370109

	// The connection with the virtual machine or container was closed.
	HCS_E_CONNECTION_CLOSED windows.Errno = 0x8037010A

	// An unknown internal message was received by the virtual machine or container.
	HCS_E_UNKNOWN_MESSAGE windows.Errno = 0x8037010B

	// The virtual machine or container does not support an available version of the communication protocol with the host.
	HCS_E_UNSUPPORTED_PROTOCOL_VERSION windows.Errno = 0x8037010C

	// The virtual machine or container JSON document is invalid.
	HCS_E_INVALID_JSON windows.Errno = 0x8037010D

	// A virtual machine or container with the specified identifier does not exist.
	HCS_E_SYSTEM_NOT_FOUND windows.Errno = 0x8037010E

	// A virtual machine or container with the specified identifier already exists.
	HCS_E_SYSTEM_ALREADY_EXISTS windows.Errno = 0x8037010F

	// The virtual machine or container with the specified identifier is not running.
	HCS_E_SYSTEM_ALREADY_STOPPED windows.Errno = 0x80370110

	// A communication protocol error has occurred between the virtual machine or container and the host.
	HCS_E_PROTOCOL_ERROR windows.Errno = 0x80370111

	// The container image contains a layer with an unrecognized format.
	HCS_E_INVALID_LAYER windows.Errno = 0x80370112

	// To use this container image, you must join the Windows Insider Program.
	// Please see https://go.microsoft.com/fwlink/?linkid=850659 for more information.
	HCS_E_WINDOWS_INSIDER_REQUIRED windows.Errno = 0x80370113

	// The operation could not be started because a required feature is not installed.
	HCS_E_SERVICE_NOT_AVAILABLE windows.Errno = 0x80370114

	// The operation has not started.
	HCS_E_OPERATION_NOT_STARTED windows.Errno = 0x80370115

	// The operation is already running.
	HCS_E_OPERATION_ALREADY_STARTED windows.Errno = 0x80370116

	// The operation is still running.
	HCS_E_OPERATION_PENDING windows.Errno = 0x80370117

	// The operation did not complete in time.
	HCS_E_OPERATION_TIMEOUT windows.Errno = 0x80370118

	// An event callback has already been registered on this handle.
	HCS_E_OPERATION_SYSTEM_CALLBACK_ALREADY_SET windows.Errno = 0x80370119

	// Not enough memory available to return the result of the operation.
	HCS_E_OPERATION_RESULT_ALLOCATION_FAILED windows.Errno = 0x8037011A

	// Insufficient privileges.
	// Only administrators or users that are members of the Hyper-V Administrators user group are permitted to access virtual machines or containers.
	// To add yourself to the Hyper-V Administrators user group, please see https://aka.ms/hcsadmin for more information.
	HCS_E_ACCESS_DENIED windows.Errno = 0x8037011B

	// The virtual machine or container reported a critical error and was stopped or restarted.
	HCS_E_GUEST_CRITICAL_ERROR windows.Errno = 0x8037011C

	// The process information is not available.
	HCS_E_PROCESS_INFO_NOT_AVAILABLE windows.Errno = 0x8037011D

	// The host compute system service has disconnected unexpectedly.
	HCS_E_SERVICE_DISCONNECT windows.Errno = 0x8037011E

	// The process has already exited.
	HCS_E_PROCESS_ALREADY_STOPPED windows.Errno = 0x8037011F

	// The virtual machine or container is not configured to perform the operation.
	HCS_E_SYSTEM_NOT_CONFIGURED_FOR_OPERATION windows.Errno = 0x80370120

	// The operation has already been cancelled.
	HCS_E_OPERATION_ALREADY_CANCELLED windows.Errno = 0x80370121
)
