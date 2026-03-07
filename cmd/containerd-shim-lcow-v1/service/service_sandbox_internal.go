//go:build windows

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	runhcsoptions "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/internal/builder/vm/lcow"
	"github.com/Microsoft/hcsshim/internal/controller/vm"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	vmsandbox "github.com/Microsoft/hcsshim/sandbox-spec-v2/vm"
	"github.com/containerd/typeurl/v2"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/go-winio"
	"github.com/containerd/containerd/api/runtime/sandbox/v1"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/errdefs"
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
func (s *Service) createSandboxInternal(ctx context.Context, sandboxID string, bundlePath string, sandboxSpec vmsandbox.Spec, options *runhcsoptions.Options) (*sandbox.CreateSandboxResponse, error) {
	s.mu.Lock()
	if s.sandboxID != "" {
		return nil, fmt.Errorf("failed to create sandbox: sandbox already exists with ID %s", s.sandboxID)
	}

	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.UVMID, sandboxID))

	// By setting the sandboxID here, we ensure that any parallel calls for CreateSandbox
	// will fail with an error.
	s.sandboxID = sandboxID
	s.mu.Unlock()

	// Use the shim binary name as the HCS owner, matching the convention used elsewhere in hcsshim.
	owner := filepath.Base(os.Args[0])

	hcsDocument, sandboxOptions, err := lcow.BuildSandboxConfig(ctx, owner, bundlePath, options, sandboxSpec.Annotations, sandboxSpec.Devices) //vmbuilder.ParseSpecs(ctx, owner, sandboxSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sandbox spec: %w", err)
	}

	s.vmHcsDocument = hcsDocument
	s.sandboxOptions = sandboxOptions

	err = s.vmController.CreateVM(ctx, &vm.CreateOptions{
		ID:          fmt.Sprintf("%s@vm", sandboxID),
		HCSDocument: hcsDocument,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create VM: %w", err)
	}

	return &sandbox.CreateSandboxResponse{}, nil
}

// startSandboxInternal is the implementation for StartSandbox.
//
// It instructs the vmController to start the VM. If the
// sandbox was created with confidential settings, confidential options are
// applied to the VM after starting.
func (s *Service) startSandboxInternal(ctx context.Context, sandboxID string) (*sandbox.StartSandboxResponse, error) {
	if s.sandboxID != sandboxID {
		return nil, fmt.Errorf("failed to start sandbox: sandbox ID mismatch, expected %s, got %s", s.sandboxID, sandboxID)
	}

	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.UVMID, sandboxID))

	var confidentialOpts *guestresource.ConfidentialOptions
	if s.sandboxOptions.ConfidentialConfig != nil {
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
// It returns the guest OS and CPU architecture for the running sandbox.
// An error is returned if the sandbox is not currently in the running state.
func (s *Service) platformInternal(ctx context.Context, sandboxID string) (*sandbox.PlatformResponse, error) {
	if s.sandboxID != sandboxID {
		return nil, fmt.Errorf("failed to get platform: sandbox ID mismatch, expected %s, got %s", s.sandboxID, sandboxID)
	}

	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.UVMID, sandboxID))

	if s.vmController.State() != vm.StateRunning {
		return nil, fmt.Errorf("failed to get platform: sandbox is not running (state: %s)", s.vmController.State())
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
func (s *Service) stopSandboxInternal(ctx context.Context, sandboxID string) (*sandbox.StopSandboxResponse, error) {
	if s.sandboxID != sandboxID {
		return nil, fmt.Errorf("failed to stop sandbox: sandbox ID mismatch, expected %s, got %s", s.sandboxID, sandboxID)
	}

	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.UVMID, sandboxID))

	err := s.vmController.TerminateVM(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to terminate VM: %w", err)
	}

	if s.vmHcsDocument.VirtualMachine.GuestState != nil {
		if err := os.Remove(s.vmHcsDocument.VirtualMachine.GuestState.GuestStateFilePath); err != nil {
			log.G(ctx).WithField("VMGS File", s.vmHcsDocument.VirtualMachine.GuestState.GuestStateFilePath).
				WithError(err).Error("failed to remove VMGS file")
		}
	}

	return &sandbox.StopSandboxResponse{}, nil
}

// waitSandboxInternal is the implementation for WaitSandbox.
//
// It blocks until the underlying VM has stopped, then maps the stopped status
// to a sandbox exit code.
func (s *Service) waitSandboxInternal(ctx context.Context, sandboxID string) (*sandbox.WaitSandboxResponse, error) {
	if s.sandboxID != sandboxID {
		return nil, fmt.Errorf("failed to wait for sandbox: sandbox ID mismatch, expected %s, got %s", s.sandboxID, sandboxID)
	}

	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.UVMID, sandboxID))

	// Wait for the VM to stop, then return the exit code.
	err := s.vmController.Wait(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for VM: %w", err)
	}

	stoppedStatus, err := s.vmController.StoppedStatus()
	if err != nil {
		return nil, fmt.Errorf("failed to get sandbox stopped status: %w", err)
	}

	exitStatus := 0
	// If there was an exit error, set a non-zero exit status.
	if stoppedStatus.Err != nil {
		exitStatus = int(windows.ERROR_INTERNAL_ERROR)
	}

	return &sandbox.WaitSandboxResponse{
		ExitStatus: uint32(exitStatus),
		ExitedAt:   timestamppb.New(stoppedStatus.StoppedTime),
	}, nil
}

// sandboxStatusInternal is the implementation for SandboxStatus.
//
// It synthesizes a status response from the current vmController state.
// When verbose is true, the response may be extended with additional
// diagnostic information.
func (s *Service) sandboxStatusInternal(_ context.Context, sandboxID string, verbose bool) (*sandbox.SandboxStatusResponse, error) {
	if s.sandboxID != sandboxID {
		return nil, fmt.Errorf("failed to get sandbox status: sandbox ID mismatch, expected %s, got %s", s.sandboxID, sandboxID)
	}

	resp := &sandbox.SandboxStatusResponse{
		SandboxID: sandboxID,
		State:     SandboxStateNotReady,
	}

	if s.vmController.State() == vm.StateNotCreated || s.vmController.State() == vm.StateCreated {
		return resp, nil
	}

	resp.CreatedAt = timestamppb.New(s.vmController.StartTime())

	if s.vmController.State() == vm.StateRunning {
		resp.State = SandboxStateReady
	}

	if s.vmController.State() == vm.StateStopped {
		stoppedStatus, err := s.vmController.StoppedStatus()
		if err != nil {
			return nil, fmt.Errorf("failed to get sandbox stopped status: %w", err)
		}
		resp.ExitedAt = timestamppb.New(stoppedStatus.StoppedTime)
	}

	if verbose {
		// Add compat info and any other detail
		// resp.Info map[string]string
		// resp.Extra any
	}

	return resp, nil
}

// pingSandboxInternal is the implementation for PingSandbox.
//
// Ping is not yet implemented for this shim.
func (s *Service) pingSandboxInternal(_ context.Context, _ string) (*sandbox.PingResponse, error) {
	// This functionality is not yet applicable for this shim.
	// Best scenario, we can return true if the VM is running.
	return nil, errdefs.ErrNotImplemented
}

// shutdownSandboxInternal is used to trigger sandbox shutdown when the shim receives
// a shutdown request from containerd.
//
// The sandbox must already be in the stopped state before shutdown is accepted.
func (s *Service) shutdownSandboxInternal(ctx context.Context, sandboxID string) (*sandbox.ShutdownSandboxResponse, error) {
	if sandboxID != s.sandboxID {
		return &sandbox.ShutdownSandboxResponse{}, fmt.Errorf("failed to shutdown sandbox: sandbox ID mismatch, expected %s, got %s", s.sandboxID, sandboxID)
	}

	if s.vmController.State() != vm.StateStopped {
		return &sandbox.ShutdownSandboxResponse{}, fmt.Errorf("failed to shutdown sandbox: sandbox is not stopped (state: %s)", s.vmController.State())
	}

	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.UVMID, sandboxID))

	// Use a goroutine to wait for the context to be done.
	// This allows us to return the response of the shutdown call prior to
	// the server being shut down.
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
func (s *Service) sandboxMetricsInternal(ctx context.Context, sandboxID string) (*sandbox.SandboxMetricsResponse, error) {
	if sandboxID != s.sandboxID {
		return &sandbox.SandboxMetricsResponse{}, fmt.Errorf("failed to get sandbox metrics: sandbox ID mismatch, expected %s, got %s", s.sandboxID, sandboxID)
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
			ID:        sandboxID,
			Data:      typeurl.MarshalProto(anyStat),
		},
	}, nil
}
