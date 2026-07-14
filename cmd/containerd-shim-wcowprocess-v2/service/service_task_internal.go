//go:build windows && wcowprocess

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/controller/process"
	container "github.com/Microsoft/hcsshim/internal/controller/windowscontainer/processisolated"
	hcs "github.com/Microsoft/hcsshim/internal/hcs/v2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oci"

	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/api/runtime/task/v3"
	"github.com/containerd/errdefs"
	"github.com/containerd/typeurl/v2"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"
)

// getContainerController looks up the container controller for the given
// container ID. It is a package-level variable so tests can substitute a mock.
var getContainerController = func(s *Service, containerID string) (containerController, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.pod == nil {
		return nil, fmt.Errorf("sandbox not created: %w", errdefs.ErrNotFound)
	}
	ctrCtrl, err := s.pod.GetContainer(containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container controller for container %s: %w: %w", containerID, errdefs.ErrNotFound, err)
	}
	return ctrCtrl, nil
}

// stateInternal returns the current status of a process within a container.
func (s *Service) stateInternal(_ context.Context, request *task.StateRequest) (*task.StateResponse, error) {
	ctrCtrl, err := getContainerController(s, request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for state request: %w", err)
	}

	proc, err := ctrCtrl.GetProcess(request.ExecID)
	if err != nil {
		return nil, fmt.Errorf("failed to get process (execID=%q) in container %s: %w: %w", request.ExecID, request.ID, errdefs.ErrNotFound, err)
	}
	return proc.Status(true), nil
}

// createInternal creates the sandbox container or a workload container.
func (s *Service) createInternal(ctx context.Context, request *task.CreateTaskRequest) (*task.CreateTaskResponse, error) {
	var spec specs.Spec
	f, err := os.Open(filepath.Join(request.Bundle, "config.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to open config.json: %w", err)
	}
	if err := json.NewDecoder(f).Decode(&spec); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("failed to decode config.json: %w", err)
	}
	_ = f.Close()

	ct, sid, err := oci.GetSandboxTypeAndID(spec.Annotations)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pod == nil {
		return nil, fmt.Errorf("sandbox %s not created: %w", sid, errdefs.ErrFailedPrecondition)
	}

	// Single-pod shim: accept the pause (sandbox) and workload containers. The CRI
	// sandbox-id annotation is not compared (azcri prefixes the sandbox object id with "uvm-").
	if ct != oci.KubernetesContainerTypeSandbox && ct != oci.KubernetesContainerTypeContainer {
		return nil, fmt.Errorf("unsupported container type %q: %w", ct, errdefs.ErrInvalidArgument)
	}

	ctrCtrl, err := s.pod.NewContainer(ctx, request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to create container %s in pod %s: %w", request.ID, sid, err)
	}

	if err := ctrCtrl.Create(ctx, &spec, request, &container.CreateOpts{}); err != nil {
		return nil, fmt.Errorf("failed to create container %s: %w", request.ID, err)
	}

	initProc, err := ctrCtrl.GetProcess("")
	if err != nil {
		return nil, fmt.Errorf("failed to get init process for container %s: %w", request.ID, err)
	}

	s.send(&eventstypes.TaskCreate{
		ContainerID: request.ID,
		Bundle:      request.Bundle,
		Rootfs:      request.Rootfs,
		IO: &eventstypes.TaskIO{
			Stdin:    request.Stdin,
			Stdout:   request.Stdout,
			Stderr:   request.Stderr,
			Terminal: request.Terminal,
		},
		Pid: uint32(initProc.Pid()),
	})

	return &task.CreateTaskResponse{
		Pid: uint32(initProc.Pid()),
	}, nil
}

// startInternal starts the init process of a container or an exec process within it.
func (s *Service) startInternal(ctx context.Context, request *task.StartRequest) (*task.StartResponse, error) {
	ctrCtrl, err := getContainerController(s, request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for start request: %w", err)
	}

	resp := &task.StartResponse{}

	if request.ExecID == "" {
		pid, err := ctrCtrl.Start(ctx, s.events)
		if err != nil {
			return nil, fmt.Errorf("failed to start container %s: %w", request.ID, err)
		}
		resp.Pid = pid
		s.send(&eventstypes.TaskStart{
			ContainerID: request.ID,
			Pid:         pid,
		})
		return resp, nil
	}

	proc, err := ctrCtrl.GetProcess(request.ExecID)
	if err != nil {
		return nil, fmt.Errorf("failed to get process (execID=%q) in container %s: %w", request.ExecID, request.ID, err)
	}

	p, err := proc.Start(ctx, s.events)
	if err != nil {
		return nil, fmt.Errorf("failed to start process (execID=%q) in container %s: %w", request.ExecID, request.ID, err)
	}
	resp.Pid = uint32(p)

	s.send(&eventstypes.TaskExecStarted{
		ContainerID: request.ID,
		ExecID:      request.ExecID,
		Pid:         uint32(p),
	})

	return resp, nil
}

// deleteInternal deletes a process, container, or the sandbox container.
func (s *Service) deleteInternal(ctx context.Context, request *task.DeleteRequest) (*task.DeleteResponse, error) {
	ctrCtrl, err := getContainerController(s, request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for delete request: %w", err)
	}

	status, err := ctrCtrl.DeleteProcess(ctx, request.ExecID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete process (execID=%q) in container %s: %w", request.ExecID, request.ID, err)
	}

	resp := &task.DeleteResponse{
		Pid:        status.Pid,
		ExitStatus: status.ExitStatus,
		ExitedAt:   status.ExitedAt,
	}

	s.send(&eventstypes.TaskDelete{
		ContainerID: request.ID,
		ID:          request.ExecID,
		Pid:         status.Pid,
		ExitStatus:  status.ExitStatus,
		ExitedAt:    status.ExitedAt,
	})

	if request.ExecID != "" {
		return resp, nil
	}

	if err := s.pod.DeleteContainer(ctx, request.ID); err != nil {
		return nil, fmt.Errorf("failed to delete container %s from pod: %w", request.ID, err)
	}

	return resp, nil
}

// pidsInternal returns the list of process IDs running inside the container.
func (s *Service) pidsInternal(ctx context.Context, request *task.PidsRequest) (*task.PidsResponse, error) {
	ctrCtrl, err := getContainerController(s, request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for pids request: %w", err)
	}

	pids, err := ctrCtrl.Pids(ctx)
	if err != nil {
		err = enrichNotFoundError(err)
		return nil, fmt.Errorf("failed to get pids for container %s: %w", request.ID, err)
	}

	return &task.PidsResponse{Processes: pids}, nil
}

// pauseInternal is not implemented for this shim.
func (s *Service) pauseInternal(_ context.Context, _ *task.PauseRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

// resumeInternal is not implemented for this shim.
func (s *Service) resumeInternal(_ context.Context, _ *task.ResumeRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

// checkpointInternal is not implemented for this shim.
func (s *Service) checkpointInternal(_ context.Context, _ *task.CheckpointTaskRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}

// killInternal sends a signal to a process or, when All is set, to every container.
func (s *Service) killInternal(ctx context.Context, request *task.KillRequest) (*emptypb.Empty, error) {
	ctrCtrl, err := getContainerController(s, request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for kill request: %w", err)
	}

	var workloadContainers map[string]*container.Controller
	if request.All && request.ID == s.sandboxID {
		s.mu.RLock()
		if s.pod != nil {
			workloadContainers = s.pod.ListContainers()
		}
		s.mu.RUnlock()
		delete(workloadContainers, request.ID)
	}

	killGroup := errgroup.Group{}
	for _, workloadCtr := range workloadContainers {
		killGroup.Go(func() error {
			return workloadCtr.KillProcess(ctx, request.ExecID, request.Signal, request.All)
		})
	}
	killGroup.Go(func() error {
		return ctrCtrl.KillProcess(ctx, request.ExecID, request.Signal, request.All)
	})

	if err = killGroup.Wait(); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// execInternal creates a new exec process inside the specified container.
func (s *Service) execInternal(ctx context.Context, request *task.ExecProcessRequest) (*emptypb.Empty, error) {
	var spec specs.Process
	if err := json.Unmarshal(request.Spec.Value, &spec); err != nil {
		return nil, fmt.Errorf("unmarshal process spec: %w", err)
	}

	ctrCtrl, err := getContainerController(s, request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for exec request: %w", err)
	}

	proc, err := ctrCtrl.NewProcess(request.ExecID)
	if err != nil {
		return nil, fmt.Errorf("failed to create new process (execID=%q) in container %s: %w", request.ExecID, request.ID, err)
	}

	if err := proc.Create(ctx, &process.CreateOptions{
		Spec:     &spec,
		Terminal: request.Terminal,
		Stdin:    request.Stdin,
		Stdout:   request.Stdout,
		Stderr:   request.Stderr,
	}); err != nil {
		return nil, fmt.Errorf("failed to create exec process (execID=%q) in container %s: %w", request.ExecID, request.ID, err)
	}

	s.send(&eventstypes.TaskExecAdded{
		ContainerID: request.ID,
		ExecID:      request.ExecID,
	})

	return &emptypb.Empty{}, nil
}

// resizePtyInternal resizes the pseudo-terminal for the specified process.
func (s *Service) resizePtyInternal(ctx context.Context, request *task.ResizePtyRequest) (*emptypb.Empty, error) {
	ctrCtrl, err := getContainerController(s, request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for resize pty request: %w", err)
	}

	proc, err := ctrCtrl.GetProcess(request.ExecID)
	if err != nil {
		return nil, fmt.Errorf("failed to get process (execID=%q) in container %s: %w", request.ExecID, request.ID, err)
	}

	if err := proc.ResizeConsole(ctx, request.Width, request.Height); err != nil {
		return nil, fmt.Errorf("failed to resize pty for process (execID=%q) in container %s: %w", request.ExecID, request.ID, err)
	}

	return &emptypb.Empty{}, nil
}

// closeIOInternal closes the stdin stream for the specified process.
func (s *Service) closeIOInternal(ctx context.Context, request *task.CloseIORequest) (*emptypb.Empty, error) {
	ctrCtrl, err := getContainerController(s, request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for close IO request: %w", err)
	}

	proc, err := ctrCtrl.GetProcess(request.ExecID)
	if err != nil {
		return nil, fmt.Errorf("failed to get process (execID=%q) in container %s: %w", request.ExecID, request.ID, err)
	}

	proc.CloseIO(ctx)
	return &emptypb.Empty{}, nil
}

// updateInternal applies a resource update to an individual container.
func (s *Service) updateInternal(ctx context.Context, request *task.UpdateTaskRequest) (*emptypb.Empty, error) {
	if request.Resources == nil {
		return nil, fmt.Errorf("update container %s: resources cannot be empty: %w", request.ID, errdefs.ErrInvalidArgument)
	}

	resources, err := typeurl.UnmarshalAny(request.Resources)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal resources for container %s update request: %w", request.ID, err)
	}

	ctrCtrl, err := getContainerController(s, request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update resources for container %s: %w", request.ID, err)
	}

	if err := ctrCtrl.Update(ctx, resources); err != nil {
		return nil, fmt.Errorf("failed to update resources for container %s: %w", request.ID, err)
	}

	return &emptypb.Empty{}, nil
}

// waitInternal blocks until the specified process exits and returns its exit status.
func (s *Service) waitInternal(ctx context.Context, request *task.WaitRequest) (*task.WaitResponse, error) {
	ctrCtrl, err := getContainerController(s, request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for wait request: %w", err)
	}

	if request.ExecID == "" {
		ctrCtrl.Wait(ctx)
	}

	proc, err := ctrCtrl.GetProcess(request.ExecID)
	if err != nil {
		return nil, fmt.Errorf("failed to get process (execID=%q) in container %s: %w", request.ExecID, request.ID, err)
	}

	proc.Wait(ctx)
	status := proc.Status(true)

	return &task.WaitResponse{
		ExitStatus: status.ExitStatus,
		ExitedAt:   status.ExitedAt,
	}, nil
}

// statsInternal returns resource usage statistics for the specified container.
func (s *Service) statsInternal(ctx context.Context, request *task.StatsRequest) (*task.StatsResponse, error) {
	ctrCtrl, err := getContainerController(s, request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for stats request: %w", err)
	}

	ctrStats, err := ctrCtrl.Stats(ctx)
	if err != nil {
		err = enrichNotFoundError(err)
		return nil, fmt.Errorf("failed to get container stats for %s: %w", request.ID, err)
	}

	anyStats, err := typeurl.MarshalAny(ctrStats)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal stats: %w", err)
	}

	return &task.StatsResponse{
		Stats: typeurl.MarshalProto(anyStats),
	}, nil
}

// shutdownInternal is a no-op; shim teardown is deferred to ShutdownSandbox.
func (s *Service) shutdownInternal(ctx context.Context, request *task.ShutdownRequest) (*emptypb.Empty, error) {
	log.G(ctx).WithFields(logrus.Fields{
		logfields.SandboxID: s.sandboxID,
		logfields.ID:        request.ID,
	}).Debug("ignoring TaskService.Shutdown request")

	return &emptypb.Empty{}, nil
}

// enrichNotFoundError wraps HCS-specific "not found" errors with errdefs.ErrNotFound.
func enrichNotFoundError(err error) error {
	isNotFound := errdefs.IsNotFound(err) ||
		hcs.IsNotExist(err) ||
		hcs.IsOperationInvalidState(err) ||
		hcs.IsAccessIsDenied(err) ||
		hcs.IsErrorInvalidHandle(err)
	if isNotFound {
		return fmt.Errorf("%w: %w", errdefs.ErrNotFound, err)
	}
	return err
}
