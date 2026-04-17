//go:build windows && lcow

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	plan9Mount "github.com/Microsoft/hcsshim/internal/controller/device/plan9/mount"
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/share"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
)

// diagExecInHostInternal is the implementation for DiagExecInHost.
//
// It is used to create an exec session into the hosting UVM.
func (s *Service) diagExecInHostInternal(ctx context.Context, request *shimdiag.ExecProcessRequest) (*shimdiag.ExecProcessResponse, error) {
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	ec, err := s.vmController.ExecIntoHost(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to exec into host: %w", err)
	}

	return &shimdiag.ExecProcessResponse{ExitCode: int32(ec)}, nil
}

// diagTasksInternal is the implementation for DiagTasks.
//
// It returns all tasks running in the UVM across all pods.
func (s *Service) diagTasksInternal(_ context.Context, request *shimdiag.TasksRequest) (*shimdiag.TasksResponse, error) {
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	// Originally this method was intended to be used in a single pod setup and therefore,
	// we do not specify a TaskID in the request. Since this shim can support multiple pods,
	// we will return all tasks running in the UVM, regardless of which pod they belong to.

	resp := &shimdiag.TasksResponse{}

	// This is a diagnostic method and therefore, locking for entire duration
	// should not have performance implications in prod.
	s.mu.Lock()
	defer s.mu.Unlock()

	// For all pods, get all the containers.
	for _, podCtrl := range s.podControllers {
		containers := podCtrl.ListContainers()

		// For each container, get their processes and status.
		for containerID, ctrCtrl := range containers {
			t := &shimdiag.Task{ID: containerID}

			if request.Execs {
				processes, err := ctrCtrl.ListProcesses()
				if err != nil {
					return nil, fmt.Errorf("failed to list processes for container %s: %w", containerID, err)
				}

				for _, proc := range processes {
					status := proc.Status(false)
					t.Execs = append(t.Execs, &shimdiag.Exec{
						ID:    status.ExecID,
						State: status.Status.String(),
					})
				}
			}

			resp.Tasks = append(resp.Tasks, t)
		}
	}

	return resp, nil
}

// diagShareInternal is the implementation for DiagShare.
//
// It shares a host path into the guest VM via the plan9 controller.
func (s *Service) diagShareInternal(ctx context.Context, request *shimdiag.ShareRequest) (*shimdiag.ShareResponse, error) {
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	fileInfo, err := os.Stat(request.HostPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source path %s: %w", request.HostPath, err)
	}

	shareConfig := share.Config{
		HostPath: request.HostPath,
		ReadOnly: request.ReadOnly,
	}

	if !fileInfo.IsDir() {
		// Map the containing directory in, but restrict the share to a single file.
		hostPath, fileName := filepath.Split(request.HostPath)
		shareConfig.HostPath = hostPath
		shareConfig.Restrict = true
		shareConfig.AllowedNames = append(shareConfig.AllowedNames, fileName)
	}

	ctrl := s.vmController.Plan9Controller()

	reservationID, err := ctrl.Reserve(ctx, shareConfig, plan9Mount.Config{ReadOnly: request.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("failed to reserve plan9 resource for request %+v: %w", request, err)
	}

	_, err = ctrl.MapToGuest(ctx, reservationID)
	if err != nil {
		return nil, fmt.Errorf("failed to map guest resource for request %+v: %w", request, err)
	}

	return &shimdiag.ShareResponse{}, nil
}

// diagStacksInternal is the implementation for DiagStacks.
//
// It collects goroutine stacks from the host shim and the guest VM.
func (s *Service) diagStacksInternal(ctx context.Context) (*shimdiag.StacksResponse, error) {
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	buf := make([]byte, 4096)
	for {
		buf = buf[:runtime.Stack(buf, true)]
		if len(buf) < cap(buf) {
			break
		}
		buf = make([]byte, 2*len(buf))
	}

	timedCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp := &shimdiag.StacksResponse{Stacks: string(buf)}
	stacks, err := s.vmController.DumpStacks(timedCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to dump stacks: %w", err)
	}

	resp.GuestStacks = stacks
	return resp, nil
}
