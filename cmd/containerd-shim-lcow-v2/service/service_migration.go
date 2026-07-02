//go:build windows && lcow

package service

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/pkg/migration"

	"github.com/containerd/errdefs/pkg/errgrpc"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

// Ensure Service implements the MigrationService interface at compile time.
var _ migration.MigrationService = &Service{}

// PrepareAndExportSandbox prepares the source sandbox for live migration and
// exports an opaque config that the destination shim can use to import it.
// This method is part of the instrumentation layer and business logic is included in prepareAndExportSandboxInternal.
func (s *Service) PrepareAndExportSandbox(ctx context.Context, request *migration.PrepareAndExportSandboxRequest) (resp *migration.PrepareAndExportSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "PrepareAndExportSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute(logfields.SandboxID, s.sandboxID),
		trace.StringAttribute(logfields.SessionID, request.SessionID))

	// Set the session id in the logger context for all subsequent logs in this request.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SessionID, request.SessionID))

	r, e := s.prepareAndExportSandboxInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// ImportSandbox imports a sandbox on the destination shim from the opaque
// config produced by PrepareAndExportSandbox on the source.
// This method is part of the instrumentation layer and business logic is included in importSandboxInternal.
func (s *Service) ImportSandbox(ctx context.Context, request *migration.ImportSandboxRequest) (resp *migration.ImportSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "ImportSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute(logfields.SandboxID, s.sandboxID),
		trace.StringAttribute(logfields.SessionID, request.SessionID))

	// Set the session id in the logger context for all subsequent logs in this request.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SessionID, request.SessionID))

	r, e := s.importSandboxInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// PrepareSandbox prepares the destination-side compute system to receive the
// migrated sandbox state.
// This method is part of the instrumentation layer and business logic is included in prepareSandboxInternal.
func (s *Service) PrepareSandbox(ctx context.Context, request *migration.PrepareSandboxRequest) (resp *migration.PrepareSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "PrepareSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute(logfields.SandboxID, s.sandboxID),
		trace.StringAttribute(logfields.SessionID, request.SessionID))

	// Set the session id in the logger context for all subsequent logs in this request.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SessionID, request.SessionID))

	r, e := s.prepareSandboxInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// TransferSandbox transfers sandbox state between source and destination
// over the previously established migration transport.
// This method is part of the instrumentation layer and business logic is included in transferSandboxInternal.
func (s *Service) TransferSandbox(ctx context.Context, request *migration.TransferSandboxRequest) (resp *migration.TransferSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "TransferSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute(logfields.SandboxID, s.sandboxID),
		trace.StringAttribute(logfields.SessionID, request.SessionID))
	if request.Timeout != nil {
		span.AddAttributes(trace.Int64Attribute(logfields.Timeout, int64(request.Timeout.AsDuration())))
	}

	// Set the session id in the logger context for all subsequent logs in this request.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SessionID, request.SessionID))

	r, e := s.transferSandboxInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// FinalizeSandbox finalizes the migration on either side: stop on the
// source, resume on the destination (per the requested action).
// This method is part of the instrumentation layer and business logic is included in finalizeSandboxInternal.
func (s *Service) FinalizeSandbox(ctx context.Context, request *migration.FinalizeSandboxRequest) (resp *migration.FinalizeSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "FinalizeSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute(logfields.SandboxID, s.sandboxID),
		trace.StringAttribute(logfields.SessionID, request.SessionID),
		trace.StringAttribute(logfields.Action, request.Action.String()))

	// Set the session id in the logger context for all subsequent logs in this request.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SessionID, request.SessionID))

	r, e := s.finalizeSandboxInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Notifications streams migration progress notifications to the caller for
// the lifetime of the migration session.
// This method is part of the instrumentation layer and business logic is included in notificationsInternal.
func (s *Service) Notifications(ctx context.Context, request *migration.NotificationsRequest, server migration.Migration_NotificationsServer) (err error) {
	ctx, span := oc.StartSpan(ctx, "Notifications")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute(logfields.SandboxID, s.sandboxID),
		trace.StringAttribute(logfields.SessionID, request.SessionID))

	// Set the session id in the logger context for all subsequent logs in this request.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SessionID, request.SessionID))

	return errgrpc.ToGRPC(s.notificationsInternal(ctx, request, server))
}

// CreateDuplicateSocket duplicates a socket handle from the caller into the
// shim process for use as the migration transport.
// This method is part of the instrumentation layer and business logic is included in createDuplicateSocketInternal.
func (s *Service) CreateDuplicateSocket(ctx context.Context, request *migration.CreateDuplicateSocketRequest) (resp *migration.CreateDuplicateSocketResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "CreateDuplicateSocket")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute(logfields.SandboxID, s.sandboxID),
		trace.StringAttribute(logfields.SessionID, request.SessionID))

	// Set the session id in the logger context for all subsequent logs in this request.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SessionID, request.SessionID))

	r, e := s.createDuplicateSocketInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Cancel aborts an in-flight migration. It is mandatory on both source and
// destination to abort a migration that is already underway; it stops the
// in-flight transfer and closes the migration socket but leaves the shim in
// the migrating state until Cleanup is called.
// This method is part of the instrumentation layer and business logic is included in cancelInternal.
func (s *Service) Cancel(ctx context.Context, request *migration.CancelRequest) (resp *migration.CancelResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "Cancel")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute(logfields.SandboxID, s.sandboxID),
		trace.StringAttribute(logfields.SessionID, request.SessionID))

	// Set the session id in the logger context for all subsequent logs in this request.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SessionID, request.SessionID))

	r, e := s.cancelInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Cleanup is the terminal call of a migration on both sides, issued regardless
// of whether the migration succeeded, failed, or was cancelled. It reverts the
// migration controller's state machine back to the normal state.
// This method is part of the instrumentation layer and business logic is included in cleanupInternal.
func (s *Service) Cleanup(ctx context.Context, request *migration.CleanupRequest) (resp *migration.CleanupResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "Cleanup")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute(logfields.SandboxID, s.sandboxID),
		trace.StringAttribute(logfields.SessionID, request.SessionID))

	// Set the session id in the logger context for all subsequent logs in this request.
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SessionID, request.SessionID))

	r, e := s.cleanupInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}
