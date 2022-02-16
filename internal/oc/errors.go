package oc

import (
	"context"
	"errors"

	"go.opencensus.io/trace"
	// "github.com/Microsoft/hcsshim/internal/hcs"
)

// todo: break import cycle with "internal/hcs"

func toStatusCode(err error) uint32 {
	switch {
	case checkErrors(err, context.Canceled):
		return trace.StatusCodeCancelled
	// case checkErrors(err, hcs.ErrVmcomputeInvalidJSON):
	// 	return trace.StatusCodeInvalidArgument
	case checkErrors(err, context.DeadlineExceeded):
		return trace.StatusCodeDeadlineExceeded
	// case checkErrors(err):
	// 	return trace.StatusCodeNotFound
	// case checkErrors(err):
	// 	return trace.StatusCodeAlreadyExists
	// case checkErrors(err):
	// 	return trace.StatusCodePermissionDenied
	// case checkErrors(err):
	// 	return trace.StatusCodeResourceExhausted
	// case checkErrors(err):
	// 	return trace.StatusCodeFailedPrecondition
	// case checkErrors(err):
	// 	return trace.StatusCodeAborted
	// case checkErrors(err):
	// 	return trace.StatusCodeOutOfRange
	// case checkErrors(err):
	// 	return trace.StatusCodeUnimplemented
	// case checkErrors(err):
	// 	return trace.StatusCodeInternal
	// case checkErrors(err):
	// 	return trace.StatusCodeUnavailable
	// case checkErrors(err):
	// 	return trace.StatusCodeDataLoss
	// case checkErrors(err):
	// 	return trace.StatusCodeUnauthenticated
	default:
		return trace.StatusCodeUnknown
	}
}

func checkErrors(err error, errs ...error) bool {
	for _, e := range errs {
		if errors.Is(err, e) {
			return true
		}
	}

	return false
}
