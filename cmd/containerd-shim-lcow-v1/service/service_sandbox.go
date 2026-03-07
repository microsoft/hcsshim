//go:build windows

package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/sandbox-spec-v2/vm"

	"github.com/containerd/containerd/api/runtime/sandbox/v1"
	errdefs2 "github.com/containerd/errdefs/pkg/errgrpc"
	"github.com/containerd/typeurl/v2"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

// Ensure Service implements the TTRPCSandboxService interface at compile time.
var _ sandbox.TTRPCSandboxService = &Service{}

// CreateSandbox creates (or prepares) a new sandbox for the given SandboxID.
func (s *Service) CreateSandbox(ctx context.Context, request *sandbox.CreateSandboxRequest) (resp *sandbox.CreateSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "CreateSandbox")
	defer span.End()
	defer func() {
		oc.SetSpanStatus(span, err)
	}()

	span.AddAttributes(
		trace.StringAttribute("sandbox-id", request.SandboxID),
		trace.StringAttribute("bundle", request.BundlePath),
		trace.StringAttribute("net-ns-path", request.NetnsPath))

	// Decode the Sandbox spec passed along from CRI.
	var sandboxSpec vm.Spec
	f, err := os.Open(filepath.Join(request.BundlePath, "config.json"))
	if err != nil {
		return nil, err
	}
	if err := json.NewDecoder(f).Decode(&sandboxSpec); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()

	// options is nil when the runtime does not pass any per-sandbox options;
	// fall back to an empty Options struct in that case so later code has a
	// consistent non-nil value to work with.
	shimOpts := &runhcsopts.Options{}
	if request.Options != nil {
		v, err := typeurl.UnmarshalAny(request.Options)
		if err != nil {
			return nil, err
		}
		shimOpts = v.(*runhcsopts.Options)

		if entry := log.G(ctx); entry.Logger.IsLevelEnabled(logrus.DebugLevel) {
			entry.WithField("options", log.Format(ctx, shimOpts)).Debug("parsed runhcs runtime options")
		}
	}

	r, e := s.createSandboxInternal(ctx, request.SandboxID, request.BundlePath, sandboxSpec, shimOpts)
	return r, errdefs2.ToGRPC(e)
}

// StartSandbox transitions a previously created sandbox to the "running" state.
func (s *Service) StartSandbox(ctx context.Context, request *sandbox.StartSandboxRequest) (resp *sandbox.StartSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "StartSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("sandbox-id", request.SandboxID))

	r, e := s.startSandboxInternal(ctx, request.SandboxID)
	return r, errdefs2.ToGRPC(e)
}

// Platform returns the platform details for the sandbox ("windows/amd64" or "linux/amd64").
func (s *Service) Platform(ctx context.Context, request *sandbox.PlatformRequest) (resp *sandbox.PlatformResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "Platform")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("sandbox-id", request.SandboxID))

	r, e := s.platformInternal(ctx, request.SandboxID)
	return r, errdefs2.ToGRPC(e)
}

// StopSandbox attempts a graceful stop of the sandbox within the specified timeout.
func (s *Service) StopSandbox(ctx context.Context, request *sandbox.StopSandboxRequest) (resp *sandbox.StopSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "StopSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("sandbox-id", request.SandboxID))
	span.AddAttributes(trace.Int64Attribute("timeout-secs", int64(request.TimeoutSecs)))

	r, e := s.stopSandboxInternal(ctx, request.GetSandboxID())
	return r, errdefs2.ToGRPC(e)
}

// WaitSandbox blocks until the sandbox reaches a terminal state (stopped/errored) and returns the outcome.
func (s *Service) WaitSandbox(ctx context.Context, request *sandbox.WaitSandboxRequest) (resp *sandbox.WaitSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "WaitSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("sandbox-id", request.SandboxID))

	r, e := s.waitSandboxInternal(ctx, request.SandboxID)
	return r, errdefs2.ToGRPC(e)
}

// SandboxStatus returns current status for the sandbox, optionally verbose.
func (s *Service) SandboxStatus(ctx context.Context, request *sandbox.SandboxStatusRequest) (resp *sandbox.SandboxStatusResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "SandboxStatus")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("sandbox-id", request.SandboxID))
	span.AddAttributes(trace.BoolAttribute("verbose", request.Verbose))

	r, e := s.sandboxStatusInternal(ctx, request.SandboxID, request.Verbose)
	return r, errdefs2.ToGRPC(e)
}

// PingSandbox performs a minimal liveness check on the sandbox and returns quickly.
func (s *Service) PingSandbox(ctx context.Context, request *sandbox.PingRequest) (resp *sandbox.PingResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "PingSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("sandbox-id", request.SandboxID))

	r, e := s.pingSandboxInternal(ctx, request.SandboxID)
	return r, errdefs2.ToGRPC(e)
}

// ShutdownSandbox requests a full shim + sandbox shutdown (stronger than StopSandbox),
// typically used by the higher-level controller to tear down resources and exit the shim.
func (s *Service) ShutdownSandbox(ctx context.Context, request *sandbox.ShutdownSandboxRequest) (resp *sandbox.ShutdownSandboxResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "ShutdownSandbox")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("sandbox-id", request.SandboxID))

	r, e := s.shutdownSandboxInternal(ctx, request.SandboxID)
	return r, errdefs2.ToGRPC(e)
}

// SandboxMetrics returns runtime metrics for the sandbox (e.g., CPU/memory/IO),
// suitable for monitoring and autoscaling decisions.
func (s *Service) SandboxMetrics(ctx context.Context, request *sandbox.SandboxMetricsRequest) (resp *sandbox.SandboxMetricsResponse, err error) {
	ctx, span := oc.StartSpan(ctx, "SandboxMetrics")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("sandbox-id", request.SandboxID))

	r, e := s.sandboxMetricsInternal(ctx, request.SandboxID)
	return r, errdefs2.ToGRPC(e)
}
