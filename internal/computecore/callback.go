//go:build windows

package computecore

import "golang.org/x/sys/windows"

// Callback functions must be converted to a uintptr via [windows.NewCallback] before being
// passed to a syscall.
//
// Additionally, [windows.NewCallback] expects functions to return a uintptr result,
// so callbacks must be modified ahead of time.
//
// Create a dedicated type uintptr for each callback to ensure type safety.

// The `void* context` parameter is for an arbitrary payload to passed into the callback,
// allowing for operation-specific data to be provided to a more generic callback.

type (
	// Function type for the completion callback of an operation.
	//
	//	typedef void (CALLBACK *HCS_OPERATION_COMPLETION)(
	//	    _In_ HCS_OPERATION operation,
	//	    _In_opt_ void* context
	//	    );
	HCSOperationCompletion func(op HCSOperation, hcsContext uintptr)

	hcsOperationCompletionUintptr uintptr
)

func (f HCSOperationCompletion) asCallback() hcsOperationCompletionUintptr {
	if f == nil {
		return hcsOperationCompletionUintptr(0)
	}
	return hcsOperationCompletionUintptr(windows.NewCallback(
		func(op HCSOperation, hcsContext uintptr) uintptr {
			f(op, hcsContext)
			return 0
		},
	))
}

type (
	// Function type for compute system event callbacks.
	//
	//	typedef void (CALLBACK *HCS_EVENT_CALLBACK)(
	//	    _In_ HCS_EVENT* event,
	//	    _In_opt_ void* context
	//	    );
	HCSEventCallback func(event *HCSEvent, hcsContext uintptr)

	hcsEventCallbackUintptr uintptr
)

func (f HCSEventCallback) asCallback() hcsEventCallbackUintptr {
	if f == nil {
		return hcsEventCallbackUintptr(0)
	}
	return hcsEventCallbackUintptr(windows.NewCallback(
		// NewCallback expects a function with one uintptr-sized result
		func(event *HCSEvent, hcsContext uintptr) uintptr {
			f(event, hcsContext)
			return 0
		},
	))
}
