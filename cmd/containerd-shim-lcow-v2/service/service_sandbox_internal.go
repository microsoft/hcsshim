//go:build windows

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Microsoft/hcsshim/internal/builder/vm/lcow"
	"github.com/Microsoft/hcsshim/internal/controller/vm"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	vmsandbox "github.com/Microsoft/hcsshim/sandbox-spec/vm/v2"

	"github.com/Microsoft/go-winio"
	"github.com/containerd/containerd/api/runtime/sandbox/v1"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/errdefs"
	"github.com/containerd/typeurl/v2"
	"golang.org/x/sys/windows"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// linuxPlatform refers to the Linux guest OS platform.
	linuxPlatform = "linux"

	// SandboxStateReady indicates the sandbox is ready.
	SandboxStateReady = "SANDBOX_READY"
	// SandboxStateNotReady indicates the sandbox is not ready.
	SandboxStateNotReady = "SANDBOX_NOTREADY"
)

// createSandboxInternal is the implementation for CreateSandbox.
//
// It enforces that only one sandbox can exist per shim instance (this shim
// follows a one-sandbox-per-shim model). It builds the HCS compute-system
// document from the sandbox spec and delegates VM creation to vmController.
func (s *Service) createSandboxInternal(ctx context.Context, request *sandbox.CreateSandboxRequest) (*sandbox.CreateSandboxResponse, error) {
	// Decode the Sandbox spec passed along from CRI.
	var sandboxSpec vmsandbox.Spec
	f, err := os.Open(filepath.Join(request.BundlePath, "config.json"))
	if err != nil {
		return nil, err
	}
	if err := json.NewDecoder(f).Decode(&sandboxSpec); err != nil {
		_ = f.Close()
		return nil, err
	}
	_ = f.Close()

	// Decode the runtime options.
	shimOpts, err := vmutils.UnmarshalRuntimeOptions(ctx, request.Options)
	if err != nil {
		return nil, err
	}

	// We take a lock at this point so that if there are multiple parallel calls to CreateSandbox,
	// only one will succeed in creating the sandbox. The successful caller will set the sandboxID,
	// which will cause the other call(s) to fail with an error indicating that a sandbox already exists.
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sandboxID != "" {
		return nil, fmt.Errorf("failed to create sandbox: sandbox already exists with ID %s", s.sandboxID)
	}

	hcsDocument, sandboxOptions, err := lcow.BuildSandboxConfig(ctx, request.BundlePath, shimOpts, &sandboxSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sandbox spec: %w", err)
	}

	s.sandboxOptions = sandboxOptions

	err = s.vmController.CreateVM(ctx, &vm.CreateOptions{
		ID:          fmt.Sprintf("%s@vm", request.SandboxID),
		HCSDocument: hcsDocument,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create VM: %w", err)
	}

	// By setting the sandboxID here, we ensure that any parallel calls for CreateSandbox
	// will fail with an error.
	// Also, setting it here acts as a synchronization point - we know that if sandboxID is set,
	// then the VM has been created successfully and sandboxOptions has been populated.
	s.sandboxID = request.SandboxID

	return &sandbox.CreateSandboxResponse{}, nil
}

// startSandboxInternal is the implementation for StartSandbox.
//
// It instructs the vmController to start the VM. If the
// sandbox was created with confidential settings, confidential options are
// applied to the VM after starting.
func (s *Service) startSandboxInternal(ctx context.Context, request *sandbox.StartSandboxRequest) (*sandbox.StartSandboxResponse, error) {
	if s.sandboxID != request.SandboxID {
		return nil, fmt.Errorf("failed to start sandbox: sandbox ID mismatch, expected %s, got %s", s.sandboxID, request.SandboxID)
	}

	// If we successfully got past the above check, it means the sandbox was created and
	// the sandboxOptions should be populated.
	var confidentialOpts *guestresource.ConfidentialOptions
	if s.sandboxOptions != nil && s.sandboxOptions.ConfidentialConfig != nil {
		uvmReferenceInfoEncoded, err := vmutils.ParseUVMReferenceInfo(
			ctx,
			vmutils.DefaultLCOWOSBootFilesPath(),
			s.sandboxOptions.ConfidentialConfig.UvmReferenceInfoFile,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to parse UVM reference info: %w", err)
		}
		confidentialOpts = &guestresource.ConfidentialOptions{
			EnforcerType:          s.sandboxOptions.ConfidentialConfig.SecurityPolicyEnforcer,
			EncodedSecurityPolicy: s.sandboxOptions.ConfidentialConfig.SecurityPolicy,
			EncodedUVMReference:   uvmReferenceInfoEncoded,
		}
	}

	// VM controller ensures that only once of the Start call goes through.
	err := s.vmController.StartVM(ctx, &vm.StartOptions{
		GCSServiceID:        winio.VsockServiceID(prot.LinuxGcsVsockPort),
		ConfidentialOptions: confidentialOpts,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start VM: %w", err)
	}

	return &sandbox.StartSandboxResponse{
		CreatedAt: timestamppb.New(s.vmController.StartTime()),
	}, nil
}

// platformInternal is the implementation for Platform.
//
// It returns the guest OS and CPU architecture for the sandbox.
// An error is returned if the sandbox is not currently in the created state.
func (s *Service) platformInternal(_ context.Context, request *sandbox.PlatformRequest) (*sandbox.PlatformResponse, error) {
	if s.sandboxID != request.SandboxID {
		return nil, fmt.Errorf("failed to get platform: sandbox ID mismatch, expected %s, got %s", s.sandboxID, request.SandboxID)
	}

	if s.vmController.State() == vm.StateNotCreated {
		return nil, fmt.Errorf("failed to get platform: sandbox has not been created (state: %s)", s.vmController.State())
	}

	return &sandbox.PlatformResponse{
		Platform: &types.Platform{
			OS:           linuxPlatform,
			Architecture: s.sandboxOptions.Architecture,
		},
	}, nil
}

// stopSandboxInternal is the implementation for StopSandbox.
//
// It terminates the VM and performs any cleanup, if needed.
func (s *Service) stopSandboxInternal(ctx context.Context, request *sandbox.StopSandboxRequest) (*sandbox.StopSandboxResponse, error) {
	if s.sandboxID != request.SandboxID {
		return nil, fmt.Errorf("failed to stop sandbox: sandbox ID mismatch, expected %s, got %s", s.sandboxID, request.SandboxID)
	}

	err := s.vmController.TerminateVM(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to terminate VM: %w", err)
	}

	return &sandbox.StopSandboxResponse{}, nil
}

// waitSandboxInternal is the implementation for WaitSandbox.
//
// It blocks until the underlying VM has been terminated, then maps the exit status
// to a sandbox exit code.
func (s *Service) waitSandboxInternal(ctx context.Context, request *sandbox.WaitSandboxRequest) (*sandbox.WaitSandboxResponse, error) {
	if s.sandboxID != request.SandboxID {
		return nil, fmt.Errorf("failed to wait for sandbox: sandbox ID mismatch, expected %s, got %s", s.sandboxID, request.SandboxID)
	}

	// Wait for the VM to be terminated, then return the exit code.
	// This is a blocking call that will wait until the VM is stopped.
	err := s.vmController.Wait(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for VM: %w", err)
	}

	exitStatus, err := s.vmController.ExitStatus()
	if err != nil {
		return nil, fmt.Errorf("failed to get sandbox exit status: %w", err)
	}

	exitStatusCode := 0
	// If there was an exit error, set a non-zero exit status.
	if exitStatus.Err != nil {
		exitStatusCode = int(windows.ERROR_INTERNAL_ERROR)
	}

	return &sandbox.WaitSandboxResponse{
		ExitStatus: uint32(exitStatusCode),
		ExitedAt:   timestamppb.New(exitStatus.StoppedTime),
	}, nil
}

// sandboxStatusInternal is the implementation for SandboxStatus.
//
// It synthesizes a status response from the current vmController state.
// When verbose is true, the response may be extended with additional
// diagnostic information.
func (s *Service) sandboxStatusInternal(_ context.Context, request *sandbox.SandboxStatusRequest) (*sandbox.SandboxStatusResponse, error) {
	if s.sandboxID != request.SandboxID {
		return nil, fmt.Errorf("failed to get sandbox status: sandbox ID mismatch, expected %s, got %s", s.sandboxID, request.SandboxID)
	}

	resp := &sandbox.SandboxStatusResponse{
		SandboxID: request.SandboxID,
	}

	switch vmState := s.vmController.State(); vmState {
	case vm.StateNotCreated, vm.StateCreated, vm.StateInvalid:
		// VM has not started yet or is in invalid state; return the default not-ready response.
		resp.State = SandboxStateNotReady
		return resp, nil
	case vm.StateRunning:
		// VM is running, so we can report the created time and ready state.
		resp.State = SandboxStateReady
		resp.CreatedAt = timestamppb.New(s.vmController.StartTime())
	case vm.StateTerminated:
		// VM has stopped, so we can report the created time, exited time, and not-ready state.
		resp.State = SandboxStateNotReady
		resp.CreatedAt = timestamppb.New(s.vmController.StartTime())
		stoppedStatus, err := s.vmController.ExitStatus()
		if err != nil {
			return nil, fmt.Errorf("failed to get sandbox stopped status: %w", err)
		}
		resp.ExitedAt = timestamppb.New(stoppedStatus.StoppedTime)
	}

	if request.Verbose { //nolint:staticcheck
		// TODO: Add compat info and any other details.
	}

	return resp, nil
}

// pingSandboxInternal is the implementation for PingSandbox.
//
// Ping is not yet implemented for this shim.
func (s *Service) pingSandboxInternal(_ context.Context, _ *sandbox.PingRequest) (*sandbox.PingResponse, error) {
	// This functionality is not yet applicable for this shim.
	// Best scenario, we can return true if the VM is running.
	return nil, errdefs.ErrNotImplemented
}

// shutdownSandboxInternal is used to trigger sandbox shutdown when the shim receives
// a shutdown request from containerd.
//
// The sandbox must already be in the stopped state before shutdown is accepted.
func (s *Service) shutdownSandboxInternal(ctx context.Context, request *sandbox.ShutdownSandboxRequest) (*sandbox.ShutdownSandboxResponse, error) {
	if s.sandboxID != request.SandboxID {
		return &sandbox.ShutdownSandboxResponse{}, fmt.Errorf("failed to shutdown sandbox: sandbox ID mismatch, expected %s, got %s", s.sandboxID, request.SandboxID)
	}

	// Ensure the VM is terminated. If the VM is already terminated,
	// TerminateVM is a no-op, so this is safe to call regardless of the current VM state.
	if state := s.vmController.State(); state != vm.StateTerminated {
		err := s.vmController.TerminateVM(ctx)
		if err != nil {
			// Just log the error instead of returning it since this is a best effort cleanup.
			log.G(ctx).WithError(err).Error("failed to terminate VM during shutdown")
		}
	}

	// With gRPC/TTRPC, the transport later creates a child context for each incoming request,
	// and cancels that context when the handler returns or the client-side connection is dropped.
	// For the shutdown request, if we call shutdown.Shutdown() directly, the shim process exits
	// prior to the response being sent back to containerd, which causes the shutdown call to fail.
	// Therefore, use a goroutine to wait for the RPC context to be done after which
	// we can safely call shutdown.Shutdown() without risking an early process exit.
	go func() {
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond) // tiny cushion to avoid edge races

		s.shutdown.Shutdown()
	}()

	return &sandbox.ShutdownSandboxResponse{}, nil
}

// sandboxMetricsInternal is the implementation for SandboxMetrics.
//
// It collects and returns runtime statistics from the vmController.
func (s *Service) sandboxMetricsInternal(ctx context.Context, request *sandbox.SandboxMetricsRequest) (*sandbox.SandboxMetricsResponse, error) {
	if s.sandboxID != request.SandboxID {
		return &sandbox.SandboxMetricsResponse{}, fmt.Errorf("failed to get sandbox metrics: sandbox ID mismatch, expected %s, got %s", s.sandboxID, request.SandboxID)
	}

	stats, err := s.vmController.Stats(ctx)
	if err != nil {
		return &sandbox.SandboxMetricsResponse{}, fmt.Errorf("failed to get sandbox metrics: %w", err)
	}

	anyStat, err := typeurl.MarshalAny(stats)
	if err != nil {
		return &sandbox.SandboxMetricsResponse{}, fmt.Errorf("failed to marshal sandbox metrics: %w", err)
	}

	return &sandbox.SandboxMetricsResponse{
		Metrics: &types.Metric{
			Timestamp: timestamppb.Now(),
			ID:        request.SandboxID,
			Data:      typeurl.MarshalProto(anyStat),
		},
	}, nil
}
