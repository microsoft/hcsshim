//go:build windows && wcowprocess

package service

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"

	"github.com/containerd/containerd/api/runtime/sandbox/v1"
	"github.com/containerd/errdefs/pkg/errgrpc"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

// Ensure Service implements the TTRPCSandboxService interface at compile time.
var _ sandbox.TTRPCSandboxService = &Service{}

// CreateSandbox creates a new sandbox for the given SandboxID.
func (s *Service) CreateSandbox(ctx context.Context, request *sandbox.CreateSandboxRequest) (resp *sandbox.CreateSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "CreateSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute(logfields.SandboxID, request.SandboxID),
		trace.StringAttribute(logfields.Bundle, request.BundlePath),
		trace.StringAttribute(logfields.NetNsPath, request.NetnsPath),
	)

	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))

	r, e := s.createSandboxInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// StartSandbox transitions a previously created sandbox to the "running" state.
func (s *Service) StartSandbox(ctx context.Context, request *sandbox.StartSandboxRequest) (resp *sandbox.StartSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "StartSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute(logfields.SandboxID, request.SandboxID))
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))

	r, e := s.startSandboxInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// Platform returns the platform details for the sandbox.
func (s *Service) Platform(ctx context.Context, request *sandbox.PlatformRequest) (resp *sandbox.PlatformResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "Platform")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute(logfields.SandboxID, request.SandboxID))

	r, e := s.platformInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// StopSandbox attempts a graceful stop of the sandbox.
func (s *Service) StopSandbox(ctx context.Context, request *sandbox.StopSandboxRequest) (resp *sandbox.StopSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "StopSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute(logfields.SandboxID, request.SandboxID))
	span.AddAttributes(trace.Int64Attribute(logfields.Timeout, int64(request.TimeoutSecs)))
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))

	r, e := s.stopSandboxInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// WaitSandbox blocks until the sandbox reaches a terminal state.
func (s *Service) WaitSandbox(ctx context.Context, request *sandbox.WaitSandboxRequest) (resp *sandbox.WaitSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "WaitSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute(logfields.SandboxID, request.SandboxID))
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))

	r, e := s.waitSandboxInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// SandboxStatus returns current status for the sandbox.
func (s *Service) SandboxStatus(ctx context.Context, request *sandbox.SandboxStatusRequest) (resp *sandbox.SandboxStatusResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "SandboxStatus")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute(logfields.SandboxID, request.SandboxID))
	span.AddAttributes(trace.BoolAttribute(logfields.Verbose, request.Verbose))
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))

	r, e := s.sandboxStatusInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// PingSandbox performs a minimal liveness check on the sandbox.
func (s *Service) PingSandbox(ctx context.Context, request *sandbox.PingRequest) (resp *sandbox.PingResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "PingSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute(logfields.SandboxID, request.SandboxID))
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))

	r, e := s.pingSandboxInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// ShutdownSandbox requests a full shim + sandbox shutdown.
func (s *Service) ShutdownSandbox(ctx context.Context, request *sandbox.ShutdownSandboxRequest) (resp *sandbox.ShutdownSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "ShutdownSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute(logfields.SandboxID, request.SandboxID))
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))

	r, e := s.shutdownSandboxInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}

// SandboxMetrics returns runtime metrics for the sandbox.
func (s *Service) SandboxMetrics(ctx context.Context, request *sandbox.SandboxMetricsRequest) (resp *sandbox.SandboxMetricsResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "SandboxMetrics")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute(logfields.SandboxID, request.SandboxID))
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))

	r, e := s.sandboxMetricsInternal(ctx, request)
	return r, errgrpc.ToGRPC(e)
}
