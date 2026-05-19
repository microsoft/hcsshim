//go:build windows

package hcs

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/computecore"
)

// infiniteTimeout is the milliseconds value passed to
// HcsWaitForOperationResult to wait forever (Win32 INFINITE, 0xFFFFFFFF).
const infiniteTimeout = ^uint32(0)

// runOperation creates an HCS operation, invokes fn(op), then synchronously
// waits for the operation result. The returned resultDoc is the JSON document
// produced by the tracked HCS API (which on failure may contain a ResultError
// describing the error events). The operation is always closed before return.
//
// ctx is honored only for tracing/logging values. Cancellation is stripped
// via context.WithoutCancel because abandoning the syscall while HCS still
// owns the operation handle leads to use-after-free crashes
// (EXCEPTION_ACCESS_VIOLATION) inside computecore.dll. Callers must not
// rely on ctx to bound the call's duration.
func runOperation(ctx context.Context, fn func(op computecore.HcsOperation) error) (resultDoc string, err error) {
	syscallCtx := context.WithoutCancel(ctx)

	// We do not use any operation level callback.
	op, err := computecore.HcsCreateOperation(syscallCtx, 0, 0)
	if err != nil {
		return "", err
	}
	defer computecore.HcsCloseOperation(syscallCtx, op)

	if fnErr := fn(op); fnErr != nil {
		// Attach any result doc HCS already produced for additional context.
		doc, _ := computecore.HcsGetOperationResult(syscallCtx, op)
		return doc, fnErr
	}
	return computecore.HcsWaitForOperationResult(syscallCtx, op, infiniteTimeout)
}

// runProcessOperation is the equivalent of runOperation for HCS APIs that are
// associated with an HCS_PROCESS handle (HcsCreateProcess, HcsGetProcessInfo,
// etc.) and whose operation result includes the HcsProcessInformation struct.
func runProcessOperation(ctx context.Context, fn func(op computecore.HcsOperation) error) (info computecore.HcsProcessInformation, resultDoc string, err error) {
	syscallCtx := context.WithoutCancel(ctx)

	op, err := computecore.HcsCreateOperation(syscallCtx, 0, 0)
	if err != nil {
		return info, "", err
	}
	defer computecore.HcsCloseOperation(syscallCtx, op)

	if fnErr := fn(op); fnErr != nil {
		_, doc, _ := computecore.HcsGetOperationResultAndProcessInfo(syscallCtx, op)
		return info, doc, fnErr
	}
	return computecore.HcsWaitForOperationResultAndProcessInfo(syscallCtx, op, infiniteTimeout)
}
