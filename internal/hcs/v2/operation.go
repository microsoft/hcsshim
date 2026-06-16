//go:build windows

package hcsv2

import (
	"context"
	"errors"
	"syscall"
	"time"

	"github.com/Microsoft/hcsshim/internal/computecore"
)

// infiniteTimeout is the milliseconds value passed to
// HcsWaitForOperationResult to wait forever (Win32 INFINITE, 0xFFFFFFFF).
const infiniteTimeout = ^uint32(0)

// hcsErrOperationTimeout is HCS_E_OPERATION_TIMEOUT, returned by
// HcsWaitForOperationResult when the wait elapses while HCS is still
// tracking the operation.
const hcsErrOperationTimeout = syscall.Errno(0x80370118)

// waitTimeoutMs derives the TimeoutMs argument for HcsWaitForOperationResult
// from ctx's deadline. With no deadline (or one already past) it falls back
// to INFINITE / a minimal poll respectively.
func waitTimeoutMs(ctx context.Context) uint32 {
	dl, ok := ctx.Deadline()
	if !ok {
		return infiniteTimeout
	}
	d := time.Until(dl)
	if d <= 0 {
		return 0
	}
	ms := d.Milliseconds()
	if ms >= int64(infiniteTimeout) {
		return infiniteTimeout - 1
	}
	return uint32(ms)
}

// runOperation creates an HCS operation, invokes fn(op), then synchronously
// waits for the operation result. The returned resultDoc is the JSON document
// produced by the tracked HCS API (which on failure may contain a ResultError
// describing the error events). The operation is always closed before return.
//
// The wait is bounded by ctx's deadline (if any); otherwise it waits forever.
// ctx cancellation is stripped via context.WithoutCancel; see computecore.execute
// for why HCS syscalls cannot be abandoned mid-flight.
func runOperation(ctx context.Context, fn func(op computecore.HcsOperation) error) (resultDoc string, err error) {
	timeoutMs := waitTimeoutMs(ctx)
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
		return doc, wrapHcsResult(ctx, fnErr, doc)
	}
	doc, waitErr := computecore.HcsWaitForOperationResult(syscallCtx, op, timeoutMs)
	if errors.Is(waitErr, hcsErrOperationTimeout) {
		// Wait deadline elapsed but HCS is still tracking the request;
		// ask it to abort so the operation does not outlive this call.
		_ = computecore.HcsCancelOperation(syscallCtx, op)
	}
	return doc, wrapHcsResult(ctx, waitErr, doc)
}

// runProcessOperation is the equivalent of runOperation for HCS APIs that are
// associated with an HCS_PROCESS handle (HcsCreateProcess, HcsGetProcessInfo,
// etc.) and whose operation result includes the HcsProcessInformation struct.
func runProcessOperation(ctx context.Context, fn func(op computecore.HcsOperation) error) (info computecore.HcsProcessInformation, resultDoc string, err error) {
	timeoutMs := waitTimeoutMs(ctx)
	syscallCtx := context.WithoutCancel(ctx)

	op, err := computecore.HcsCreateOperation(syscallCtx, 0, 0)
	if err != nil {
		return info, "", err
	}
	defer computecore.HcsCloseOperation(syscallCtx, op)

	if fnErr := fn(op); fnErr != nil {
		_, doc, _ := computecore.HcsGetOperationResultAndProcessInfo(syscallCtx, op)
		return info, doc, wrapHcsResult(ctx, fnErr, doc)
	}
	info, doc, waitErr := computecore.HcsWaitForOperationResultAndProcessInfo(syscallCtx, op, timeoutMs)
	if errors.Is(waitErr, hcsErrOperationTimeout) {
		_ = computecore.HcsCancelOperation(syscallCtx, op)
	}
	return info, doc, wrapHcsResult(ctx, waitErr, doc)
}

// submitOperation creates an operation, invokes fn(op) to hand the request
// to HCS, and returns without waiting for completion. The operation handle
// is closed immediately; per the HCS V2 contract this is safe while the
// request is in flight (HCS continues running it and discards the result).
//
// This preserves the V1 fire-and-forget semantics of HcsShutdownComputeSystem
// and HcsTerminateComputeSystem, where the shim only needs to know the
// request was accepted (callers observe completion via the system exit
// notification, not the operation result).
func submitOperation(ctx context.Context, fn func(op computecore.HcsOperation) error) error {
	syscallCtx := context.WithoutCancel(ctx)
	op, err := computecore.HcsCreateOperation(syscallCtx, 0, 0)
	if err != nil {
		return err
	}
	defer computecore.HcsCloseOperation(syscallCtx, op)
	if fnErr := fn(op); fnErr != nil {
		doc, _ := computecore.HcsGetOperationResult(syscallCtx, op)
		return wrapHcsResult(ctx, fnErr, doc)
	}
	return nil
}
