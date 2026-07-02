//go:build windows && lcow

package service

import (
	"context"
	"fmt"
	"time"

	migrationcontroller "github.com/Microsoft/hcsshim/internal/controller/migration"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/pkg/migration"

	"github.com/containerd/containerd/api/runtime/task/v3"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// prepareAndExportSandboxInternal is the implementation for PrepareAndExportSandbox.
//
// It initializes the source sandbox for migration and exports the
// opaque snapshot consumed by the destination shim's ImportSandbox API.
func (s *Service) prepareAndExportSandboxInternal(ctx context.Context, request *migration.PrepareAndExportSandboxRequest) (*migration.PrepareAndExportSandboxResponse, error) {
	// Convert the protobuf migration options to the HCS representation.
	migrationOpts, err := migration.InitializeOptionsFromProto(request.InitOptions)
	if err != nil {
		return nil, fmt.Errorf("convert migration initialize options: %w", err)
	}

	// Arm the source for migration.
	if err = s.migrationController.PrepareSource(ctx,
		&migrationcontroller.PrepareSourceOptions{
			InitOptions: migrationcontroller.InitOptions{
				SessionID:      request.SessionID,
				Origin:         hcsschema.MigrationOriginSource,
				VMController:   s.vmController,
				PodControllers: s.podControllers,
			},
			MigrationOpts: migrationOpts,
		}); err != nil {
		return nil, fmt.Errorf("source: prepare migration: %w", err)
	}

	// Produce the opaque sandbox snapshot (VM plus per-pod state) that the
	// destination shim consumes through ImportSandbox.
	cfg, err := s.migrationController.ExportState(ctx, request.SessionID)
	if err != nil {
		return nil, fmt.Errorf("source: export migration state: %w", err)
	}

	return &migration.PrepareAndExportSandboxResponse{Config: cfg}, nil
}

// importSandboxInternal is the implementation for ImportSandbox.
//
// It rehydrates the destination shim from the source's opaque snapshot,
// mutating the Service-owned vm controller and pod controllers in place.
func (s *Service) importSandboxInternal(ctx context.Context, request *migration.ImportSandboxRequest) (*migration.ImportSandboxResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Rehydrate the destination from the source snapshot, recreating the VM
	// and pod state and indexing each container to its owning pod.
	if err := s.migrationController.ImportState(ctx,
		&migrationcontroller.ImportStateOptions{
			InitOptions: migrationcontroller.InitOptions{
				SessionID:      request.SessionID,
				Origin:         hcsschema.MigrationOriginDestination,
				VMController:   s.vmController,
				PodControllers: s.podControllers,
			},
			SandboxID:           request.SandboxID,
			SavedState:          request.Config,
			ContainerPodMapping: s.containerPodMapping,
		}); err != nil {
		return nil, fmt.Errorf("destination: import migration state: %w", err)
	}

	s.sandboxID = request.SandboxID
	return &migration.ImportSandboxResponse{}, nil
}

// patchMigratedContainerInternal completes CreateTask for a container already
// rehydrated by migration: it rebinds the container's host-side resources to
// this shim and returns its existing init process PID without starting anything.
func (s *Service) patchMigratedContainerInternal(
	ctx context.Context,
	request *task.CreateTaskRequest,
	spec specs.Spec,
) (*task.CreateTaskResponse, error) {
	// Rebind the migrated container's host-side resources to this side's IDs.
	err := s.migrationController.PatchResourcePaths(ctx, request, spec)
	if err != nil {
		return nil, fmt.Errorf("patch migrated container %q: %w", request.ID, err)
	}

	ctrCtrl, err := getContainerController(s, request.ID)
	if err != nil {
		return nil, fmt.Errorf("lookup migrated container %q: %w", request.ID, err)
	}

	initProc, err := ctrCtrl.GetProcess("")
	if err != nil {
		return nil, fmt.Errorf("get init process for migrated container %q: %w", request.ID, err)
	}

	return &task.CreateTaskResponse{Pid: uint32(initProc.Pid())}, nil
}

// prepareSandboxInternal is the implementation for PrepareSandbox.
//
// It creates the destination VM's HCS compute system and re-ACLs its disks so
// it is ready to start; all migrated containers must already be patched.
func (s *Service) prepareSandboxInternal(ctx context.Context, request *migration.PrepareSandboxRequest) (*migration.PrepareSandboxResponse, error) {
	migrationOpts, err := migration.InitializeOptionsFromProto(request.InitOptions)
	if err != nil {
		return nil, fmt.Errorf("convert migration initialize options: %w", err)
	}

	// Build the destination VM and prep its disks; every container must be patched first.
	if err := s.migrationController.PrepareDestination(
		ctx,
		request.SessionID,
		migrationOpts,
	); err != nil {
		return nil, fmt.Errorf("destination: prepare migration: %w", err)
	}

	return &migration.PrepareSandboxResponse{}, nil
}

// transferSandboxInternal is the implementation for TransferSandbox.
//
// It drives the memory transfer between source and destination over the
// duplicated socket.
func (s *Service) transferSandboxInternal(ctx context.Context, request *migration.TransferSandboxRequest) (*migration.TransferSandboxResponse, error) {
	var timeout time.Duration
	if request.Timeout != nil {
		timeout = request.Timeout.AsDuration()
	}

	// Wait for the transport socket, then drive the memory transfer within the timeout.
	if err := s.migrationController.Transfer(
		ctx,
		request.SessionID,
		timeout,
	); err != nil {
		return nil, fmt.Errorf("transfer migration state: %w", err)
	}
	return &migration.TransferSandboxResponse{}, nil
}

// finalizeSandboxInternal is the implementation for FinalizeSandbox.
//
// It commits the migration's final outcome on each side per the requested
// action — resuming the sandbox back to running or stopping it — and ends the session.
func (s *Service) finalizeSandboxInternal(ctx context.Context, request *migration.FinalizeSandboxRequest) (*migration.FinalizeSandboxResponse, error) {
	// Apply the requested resume/stop outcome and end the session.
	if err := s.migrationController.Finalize(ctx, request.SessionID, request.Action, s.events); err != nil {
		return nil, fmt.Errorf("finalize migration session: %w", err)
	}

	return &migration.FinalizeSandboxResponse{}, nil
}

// notificationsInternal is the implementation for Notifications.
//
// It forwards migration notifications to the calling stream until the session
// terminates or the client disconnects.
func (s *Service) notificationsInternal(ctx context.Context, request *migration.NotificationsRequest, server migration.Migration_NotificationsServer) error {
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Attach to this session's migration progress notifications.
	ch, err := s.migrationController.Subscribe(subCtx, request.SessionID)
	if err != nil {
		return fmt.Errorf("subscribe to migration notifications: %w", err)
	}

	logger := log.G(ctx).WithField(logfields.SessionID, request.SessionID)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case resp, ok := <-ch:
			if !ok {
				// Session terminated; close the stream cleanly.
				return nil
			}

			// Forward each notification to the client stream.
			if err := server.Send(resp); err != nil {
				logger.WithError(err).Warn("send migration notification failed")
				return fmt.Errorf("send migration notification: %w", err)
			}
		}
	}
}

// createDuplicateSocketInternal is the implementation for CreateDuplicateSocket.
//
// It reconstructs the connected migration transport socket from the caller's
// duplicated protocol info and adopts it for the session, unblocking the transfer.
func (s *Service) createDuplicateSocketInternal(ctx context.Context, request *migration.CreateDuplicateSocketRequest) (*migration.CreateDuplicateSocketResponse, error) {
	// Adopt the caller's duplicated socket as the transport and unblock the transfer.
	if err := s.migrationController.RegisterDuplicateSocket(ctx, request.SessionID, request.ProtocolInfo); err != nil {
		return nil, fmt.Errorf("register duplicate migration socket: %w", err)
	}

	return &migration.CreateDuplicateSocketResponse{}, nil
}

// cancelInternal is the implementation for Cancel.
//
// It aborts the in-flight transfer and closes the migration socket without
// reverting the controller state machine; Cleanup performs the final revert.
func (s *Service) cancelInternal(ctx context.Context, request *migration.CancelRequest) (*migration.CancelResponse, error) {
	// Abort the in-flight transfer now; the session stays until cleanup.
	if err := s.migrationController.Cancel(ctx, request.SessionID); err != nil {
		return nil, fmt.Errorf("cancel migration session: %w", err)
	}

	return &migration.CancelResponse{}, nil
}

// cleanupInternal is the implementation for Cleanup.
//
// It reverts the migration controller state machine back to idle, resuming or
// aborting the underlying controllers based on the current migration state.
func (s *Service) cleanupInternal(ctx context.Context, request *migration.CleanupRequest) (*migration.CleanupResponse, error) {
	// Tear down the session and revert this side back to idle.
	if err := s.migrationController.Cleanup(ctx, request.SessionID, s.events); err != nil {
		return nil, fmt.Errorf("cleanup migration session: %w", err)
	}

	return &migration.CleanupResponse{}, nil
}
