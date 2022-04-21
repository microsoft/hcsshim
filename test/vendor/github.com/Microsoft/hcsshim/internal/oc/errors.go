package oc

import (
	"context"
	"errors"
	"io"
	"net"
	"os"

	"github.com/containerd/containerd/errdefs"
	"go.opencensus.io/trace"
)

// todo: break import cycle with "internal/hcs"
// todo: unify or add in "internal/guest/gcserror"
// todo: move go-winio errors (ErrTimeout, ErrFileClosed, ErrPipeListenerClosed) to OS-neutral location

// via https://pkg.go.dev/go.opencensus.io/trace#pkg-constants

func toStatusCode(err error) uint32 {
	switch {
	case checkErrors(err, context.Canceled):
		return trace.StatusCodeCancelled
	case checkErrors(err, os.ErrInvalid, errdefs.ErrInvalidArgument):
		return trace.StatusCodeInvalidArgument
	case checkErrors(err, context.DeadlineExceeded, os.ErrDeadlineExceeded):
		return trace.StatusCodeDeadlineExceeded
	case checkErrors(err, os.ErrNotExist, errdefs.ErrNotFound):
		return trace.StatusCodeNotFound
	case checkErrors(err, os.ErrExist, errdefs.ErrAlreadyExists):
		return trace.StatusCodeAlreadyExists
	case checkErrors(err, os.ErrPermission):
		return trace.StatusCodePermissionDenied
	// case checkErrors(err):
	// 	return trace.StatusCodeResourceExhausted
	case checkErrors(err, os.ErrClosed, net.ErrClosed,
		errdefs.ErrFailedPrecondition, io.ErrClosedPipe):
		return trace.StatusCodeFailedPrecondition
	// case checkErrors(err):
	// 	return trace.StatusCodeAborted
	// case checkErrors(err):
	// 	return trace.StatusCodeOutOfRange
	case checkErrors(err, errdefs.ErrNotImplemented):
		return trace.StatusCodeUnimplemented
	// case checkErrors(err):
	// 	return trace.StatusCodeInternal
	case checkErrors(err, errdefs.ErrUnavailable):
		return trace.StatusCodeUnavailable
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
