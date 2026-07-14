//go:build windows && wcowprocess

package service

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/Microsoft/hcsshim/internal/controller/pod"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"

	"github.com/containerd/containerd/api/runtime/sandbox/v1"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/errdefs"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// windowsPlatform refers to the Windows host OS platform.
	windowsPlatform = "windows"

	// SandboxStateReady indicates the sandbox is ready.
	SandboxStateReady = "SANDBOX_READY"
	// SandboxStateNotReady indicates the sandbox is not ready.
	SandboxStateNotReady = "SANDBOX_NOTREADY"

	// sigKill is used to terminate containers during sandbox teardown.
	sigKill = 9

	// sandboxIDPrefix is prepended by azcri to the pause container ID to form the sandbox ID.
	sandboxIDPrefix = "uvm-"
)

// createSandboxInternal is the implementation for CreateSandbox. Only one
// sandbox may exist per shim instance.
func (s *Service) createSandboxInternal(ctx context.Context, request *sandbox.CreateSandboxRequest) (*sandbox.CreateSandboxResponse, error) {
	shimOpts, err := vmutils.UnmarshalRuntimeOptions(ctx, request.Options)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sandboxID != "" {
		return nil, fmt.Errorf("sandbox already exists with ID %s", s.sandboxID)
	}

	var ioRetryTimeout time.Duration
	if shimOpts != nil {
		ioRetryTimeout = time.Duration(shimOpts.IoRetryTimeoutInSec) * time.Second
	}

	s.pod = pod.New(request.SandboxID, ioRetryTimeout)
	s.sandboxID = request.SandboxID

	return &sandbox.CreateSandboxResponse{}, nil
}

// startSandboxInternal is the implementation for StartSandbox. It is a no-op for
// process-isolated pods; the sandbox container is started via the task API.
func (s *Service) startSandboxInternal(_ context.Context, request *sandbox.StartSandboxRequest) (*sandbox.StartSandboxResponse, error) {
	if s.sandboxID != request.SandboxID {
		return nil, fmt.Errorf("sandbox ID mismatch, expected %s, got %s", s.sandboxID, request.SandboxID)
	}

	return &sandbox.StartSandboxResponse{
		CreatedAt: timestamppb.Now(),
	}, nil
}

// platformInternal is the implementation for Platform.
func (s *Service) platformInternal(_ context.Context, request *sandbox.PlatformRequest) (*sandbox.PlatformResponse, error) {
	if s.sandboxID != request.SandboxID {
		return nil, fmt.Errorf("sandbox ID mismatch, expected %s, got %s", s.sandboxID, request.SandboxID)
	}

	return &sandbox.PlatformResponse{
		Platform: &types.Platform{
			OS:           windowsPlatform,
			Architecture: runtime.GOARCH,
		},
	}, nil
}

// stopSandboxInternal is the implementation for StopSandbox. It terminates every
// container in the pod.
func (s *Service) stopSandboxInternal(ctx context.Context, request *sandbox.StopSandboxRequest) (*sandbox.StopSandboxResponse, error) {
	if s.sandboxID != request.SandboxID {
		return nil, fmt.Errorf("sandbox ID mismatch, expected %s, got %s", s.sandboxID, request.SandboxID)
	}

	s.terminateContainers(ctx)
	return &sandbox.StopSandboxResponse{}, nil
}

// waitSandboxInternal is the implementation for WaitSandbox. It blocks until the
// sandbox container has terminated.
func (s *Service) waitSandboxInternal(ctx context.Context, request *sandbox.WaitSandboxRequest) (*sandbox.WaitSandboxResponse, error) {
	if s.sandboxID != request.SandboxID {
		return nil, fmt.Errorf("sandbox ID mismatch, expected %s, got %s", s.sandboxID, request.SandboxID)
	}

	// The pause container is the sandbox; block until it exits.
	if ctr, err := s.pauseContainer(); err == nil {
		ctr.Wait(ctx)
	}

	return &sandbox.WaitSandboxResponse{
		ExitStatus: 0,
		ExitedAt:   timestamppb.Now(),
	}, nil
}

// sandboxStatusInternal is the implementation for SandboxStatus.
func (s *Service) sandboxStatusInternal(_ context.Context, request *sandbox.SandboxStatusRequest) (*sandbox.SandboxStatusResponse, error) {
	if s.sandboxID != request.SandboxID {
		return nil, fmt.Errorf("sandbox ID mismatch, expected %s, got %s", s.sandboxID, request.SandboxID)
	}

	// The sandbox is ready while its pause container is present.
	resp := &sandbox.SandboxStatusResponse{SandboxID: request.SandboxID, State: SandboxStateNotReady}
	if ctr, err := s.pauseContainer(); err == nil {
		resp.State = SandboxStateReady
		resp.CreatedAt = timestamppb.Now()
		if p, perr := ctr.GetProcess(""); perr == nil {
			resp.Pid = uint32(p.Pid())
		}
	}
	return resp, nil
}

// pingSandboxInternal is not implemented for this shim.
func (s *Service) pingSandboxInternal(_ context.Context, _ *sandbox.PingRequest) (*sandbox.PingResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

// shutdownSandboxInternal terminates the pod and shuts down the shim process.
func (s *Service) shutdownSandboxInternal(ctx context.Context, request *sandbox.ShutdownSandboxRequest) (*sandbox.ShutdownSandboxResponse, error) {
	if s.sandboxID != request.SandboxID {
		return nil, fmt.Errorf("sandbox ID mismatch, expected %s, got %s", s.sandboxID, request.SandboxID)
	}

	s.terminateContainers(ctx)

	// Defer the actual shutdown until the RPC context is done so the response
	// is sent back to containerd before the process exits.
	go func() {
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond)
		s.shutdown.Shutdown()
	}()

	return &sandbox.ShutdownSandboxResponse{}, nil
}

// sandboxMetricsInternal is not implemented for this shim.
func (s *Service) sandboxMetricsInternal(_ context.Context, _ *sandbox.SandboxMetricsRequest) (*sandbox.SandboxMetricsResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

// pauseContainer resolves the pause container that backs the sandbox from the
// sandbox ID (azcri forms it as sandboxIDPrefix + the pause container ID).
func (s *Service) pauseContainer() (containerController, error) {
	s.mu.RLock()
	pod, sid := s.pod, s.sandboxID
	s.mu.RUnlock()
	if pod == nil {
		return nil, errdefs.ErrNotFound
	}
	return pod.GetContainer(strings.TrimPrefix(sid, sandboxIDPrefix))
}

// terminateContainers best-effort kills every container in the pod.
func (s *Service) terminateContainers(ctx context.Context) {
	if s.pod == nil {
		return
	}
	for id, ctr := range s.pod.ListContainers() {
		if err := ctr.KillProcess(ctx, "", sigKill, true); err != nil {
			log.G(ctx).WithError(err).WithField("container", id).Warn("failed to terminate container during sandbox teardown")
		}
	}
}
