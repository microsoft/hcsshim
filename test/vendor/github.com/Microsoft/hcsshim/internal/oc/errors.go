package oc

import (
	"io"
	"net"
	"os"

	"github.com/containerd/containerd/errdefs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Microsoft/hcsshim/internal/hcserror"
)

// todo: break import cycle with "internal/hcs" and errors errors defined there
// todo: add errors defined in "internal/guest/gcserror" (Hresult does not implement error)

func toStatusCode(err error) codes.Code {
	// checks if err implements GRPCStatus() *"google.golang.org/grpc/status".Status,
	// wraps an error defined in "github.com/containerd/containerd/errdefs", or is a
	// context timeout or cancelled error
	if s, ok := status.FromError(errdefs.ToGRPC(err)); ok {
		return s.Code()
	}

	switch {
	// case hcserror.IsAny(err):
	// 	return codes.Cancelled
	case hcserror.IsAny(err, os.ErrInvalid):
		return codes.InvalidArgument
	case hcserror.IsAny(err, os.ErrDeadlineExceeded):
		return codes.DeadlineExceeded
	case hcserror.IsAny(err, os.ErrNotExist):
		return codes.NotFound
	case hcserror.IsAny(err, os.ErrExist):
		return codes.AlreadyExists
	case hcserror.IsAny(err, os.ErrPermission):
		return codes.PermissionDenied
	// case hcserror.IsAny(err):
	// 	return codes.ResourceExhausted
	case hcserror.IsAny(err, os.ErrClosed, net.ErrClosed, io.ErrClosedPipe, io.ErrShortBuffer):
		return codes.FailedPrecondition
	// case hcserror.IsAny(err):
	// 	return codes.Aborted
	// case hcserror.IsAny(err):
	// 	return codes.OutOfRange
	// case hcserror.IsAny(err):
	// 	return codes.Unimplemented
	case hcserror.IsAny(err, io.ErrNoProgress):
		return codes.Internal
	// case hcserror.IsAny(err):
	// 	return codes.Unavailable
	case hcserror.IsAny(err, io.ErrShortWrite, io.ErrUnexpectedEOF):
		return codes.DataLoss
	// case hcserror.IsAny(err):
	// 	return codes.Unauthenticated
	default:
		return codes.Unknown
	}
}
