//go:build windows && lcow

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	container "github.com/Microsoft/hcsshim/internal/controller/linuxcontainer"
	"github.com/Microsoft/hcsshim/internal/controller/pod"
	"github.com/Microsoft/hcsshim/internal/controller/process"
	"github.com/Microsoft/hcsshim/internal/hcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/annotations"

	"github.com/Microsoft/hcsshim/pkg/ctrdtaskapi"
	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/errdefs"
	"github.com/containerd/typeurl/v2"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"
)

// getContainerController looks up the container controller for the given container ID.
func (s *Service) getContainerController(containerID string) (*container.Controller, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Resolve the owning pod ID from the container-to-pod mapping.
	podID, ok := s.containerPodMapping[containerID]
	if !ok {
		return nil, fmt.Errorf("container %s not found: %w", containerID, errdefs.ErrNotFound)
	}

	// Fetch the pod controller responsible for this pod.
	podCtrl, ok := s.podControllers[podID]
	if !ok {
		return nil, fmt.Errorf("pod controller for pod %s not found: %w", podID, errdefs.ErrNotFound)
	}

	// Retrieve the container controller from the pod.
	ctrCtrl, err := podCtrl.GetContainer(containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container controller for container %s in pod %s: %w", containerID, podID, err)
	}

	return ctrCtrl, nil
}

// getPodController returns pod controller for given pod ID.
func (s *Service) getPodController(podID string) (*pod.Controller, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctrl, ok := s.podControllers[podID]
	return ctrl, ok
}

// stateInternal returns the current status of a process within a container.
func (s *Service) stateInternal(_ context.Context, request *task.StateRequest) (*task.StateResponse, error) {
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	// Look up the container controller for the requested container.
	ctrCtrl, err := s.getContainerController(request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for state request: %w", err)
	}

	// Retrieve the process controller for the target exec (or init) process.
	proc, err := ctrCtrl.GetProcess(request.ExecID)
	if err != nil {
		return nil, fmt.Errorf("failed to get process (execID=%q) in container %s: %w: %w", request.ExecID, request.ID, errdefs.ErrNotFound, err)
	}

	// Return the current status snapshot for the process.
	return proc.Status(true), nil
}

// createInternal creates a new pod sandbox or workload container based on the OCI spec.
func (s *Service) createInternal(ctx context.Context, request *task.CreateTaskRequest) (*task.CreateTaskResponse, error) {
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	// Parse the OCI spec from the bundle.
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

	// Determine the sandbox type and ID.
	// Sandbox type can be "sandbox" or "container".
	ct, sid, err := oci.GetSandboxTypeAndID(spec.Annotations)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var ctrCtrl *container.Controller

	switch ct {
	case oci.KubernetesContainerTypeSandbox:
		// This is a pod creation request. Create a new pod controller.
		if _, ok := s.podControllers[sid]; ok {
			return nil, fmt.Errorf("pod controller for pod %s already exists: %w", sid, errdefs.ErrAlreadyExists)
		}

		// Validate that required config fields are present for a sandbox.
		if spec.Windows == nil || spec.Windows.Network == nil {
			return nil, fmt.Errorf("spec is missing required Windows network configuration: %w", errdefs.ErrInvalidArgument)
		}

		// If any unsupported param is specified, return an explicit error.
		if len(spec.Windows.Network.EndpointList) > 0 {
			return nil, fmt.Errorf("spec has unsupported network configuration: endpoints should not be part of spec: %w", errdefs.ErrInvalidArgument)
		}

		// Create a new pod.
		podCtrl := pod.New(sid, spec.Windows.Network.NetworkNamespace, s.vmController)

		// Setup network for the pod based on the provided namespace.
		err = podCtrl.SetupNetwork(ctx)
		if err != nil {
			// No cleanup on failure since containerd will send a Delete request.
			return nil, fmt.Errorf("failed to setup network for pod %s: %w", sid, err)
		}

		// Store the created controller in the map.
		s.podControllers[sid] = podCtrl

		// Create a container within the pod with the same ID as the pod.
		ctrCtrl, err = podCtrl.NewContainer(ctx, request.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to create sandbox container %s in pod %s: %w", request.ID, sid, err)
		}

		s.containerPodMapping[request.ID] = sid

	case oci.KubernetesContainerTypeContainer:
		// This is a regular container creation request. Look up the existing pod.
		podCtrl, ok := s.podControllers[sid]
		if !ok {
			return nil, fmt.Errorf("pod controller for pod %s not found: %w", sid, errdefs.ErrNotFound)
		}

		// Create a container within the pod with the provided ID.
		ctrCtrl, err = podCtrl.NewContainer(ctx, request.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to create container %s in pod %s: %w", request.ID, sid, err)
		}

		s.containerPodMapping[request.ID] = sid

	default:
		return nil, fmt.Errorf("unsupported container type %q: %w", ct, errdefs.ErrInvalidArgument)
	}

	// Get EnableScratchEncryption option.
	var enableScratchEncryption bool
	sandboxOpts := s.vmController.SandboxOptions()
	if sandboxOpts != nil {
		enableScratchEncryption = sandboxOpts.EnableScratchEncryption
	}

	// Call Create on the container controller.
	if err := ctrCtrl.Create(
		ctx,
		&spec,
		request,
		&container.CreateOpts{
			IsScratchEncryptionEnabled: enableScratchEncryption,
		},
	); err != nil {
		return nil, fmt.Errorf("failed to create container %s: %w", request.ID, err)
	}

	// Get the init process pid to return in the response.
	initProc, err := ctrCtrl.GetProcess("")
	if err != nil {
		return nil, fmt.Errorf("failed to get init process for container %s: %w", request.ID, err)
	}

	// Publish the TaskCreate event to notify containerd that the container has been created.
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
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	// Get the container controller for the requested task.
	ctrCtrl, err := s.getContainerController(request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for start request: %w", err)
	}

	resp := &task.StartResponse{}

	// If the start was meant for container,
	// call start on Container controller.
	if request.ExecID == "" {
		pid, err := ctrCtrl.Start(ctx, s.events)
		if err != nil {
			return nil, fmt.Errorf("failed to start container %s: %w", request.ID, err)
		}
		resp.Pid = pid

		// Publish the TaskStart event for the init process.
		s.send(&eventstypes.TaskStart{
			ContainerID: request.ID,
			Pid:         pid,
		})

		return resp, nil
	}

	// If the start was meant for exec process,
	// call start on Process controller.
	proc, err := ctrCtrl.GetProcess(request.ExecID)
	if err != nil {
		return nil, fmt.Errorf("failed to get process (execID=%q) in container %s: %w", request.ExecID, request.ID, err)
	}

	p, err := proc.Start(ctx, s.events)
	if err != nil {
		return nil, fmt.Errorf("failed to start process (execID=%q) in container %s: %w", request.ExecID, request.ID, err)
	}
	resp.Pid = uint32(p)

	// Publish the TaskExecStarted event for the exec process.
	s.send(&eventstypes.TaskExecStarted{
		ContainerID: request.ID,
		ExecID:      request.ExecID,
		Pid:         uint32(p),
	})

	return resp, nil
}

// deleteInternal deletes a process, container, or pod sandbox depending on the request.
func (s *Service) deleteInternal(ctx context.Context, request *task.DeleteRequest) (*task.DeleteResponse, error) {
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	// Look up the container controller for the target ID.
	ctrCtrl, err := s.getContainerController(request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for delete request: %w", err)
	}

	// Delete the process from the container controller.
	// For the init process this is request.ExecID == "".
	status, err := ctrCtrl.DeleteProcess(ctx, request.ExecID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete process (execID=%q) in container %s: %w", request.ExecID, request.ID, err)
	}

	// Build the response from the process status returned by DeleteProcess.
	resp := &task.DeleteResponse{
		Pid:        status.Pid,
		ExitStatus: status.ExitStatus,
		ExitedAt:   status.ExitedAt,
	}

	// Publish the TaskDelete event to notify containerd the process/task has been deleted.
	s.send(&eventstypes.TaskDelete{
		ContainerID: request.ID,
		ID:          request.ExecID,
		Pid:         status.Pid,
		ExitStatus:  status.ExitStatus,
		ExitedAt:    status.ExitedAt,
	})

	// If this was an exec process deletion, we are done.
	if request.ExecID != "" {
		return resp, nil
	}

	// We need to delete either a pod or a container.
	s.mu.Lock()
	defer s.mu.Unlock()

	podID := s.containerPodMapping[request.ID]

	// If the container ID matches a pod ID, this is the sandbox container
	// being torn down.
	if podCtrl, isPod := s.podControllers[request.ID]; isPod {
		// Ensure no workload containers remain in the pod. The only container
		// left should be the sandbox container itself (request.ID).
		remaining := podCtrl.ListContainers()
		delete(remaining, request.ID) // exclude the sandbox container itself
		if len(remaining) > 0 {
			return nil, fmt.Errorf("cannot delete sandbox container %s: %d workload container(s) still exist in the pod: %w",
				request.ID, len(remaining), errdefs.ErrFailedPrecondition)
		}

		// Tear down the pod network before removing the pod controller.
		if err := podCtrl.TeardownNetwork(ctx); err != nil {
			return nil, fmt.Errorf("failed to teardown network for pod %s: %w", request.ID, err)
		}

		// Remove the sandbox container from the pod's internal container map.
		if err := podCtrl.DeleteContainer(ctx, request.ID); err != nil {
			return nil, fmt.Errorf("failed to delete sandbox container %s from pod: %w", request.ID, err)
		}

		delete(s.podControllers, request.ID)
		delete(s.containerPodMapping, request.ID)
		return resp, nil
	}

	// Regular (non-sandbox) container: delete the container from the owning
	// pod controller first, then remove the mapping.
	podCtrl, ok := s.podControllers[podID]
	if !ok {
		return nil, fmt.Errorf("pod controller for pod %s not found while deleting container %s: %w", podID, request.ID, errdefs.ErrNotFound)
	}

	if err := podCtrl.DeleteContainer(ctx, request.ID); err != nil {
		return nil, fmt.Errorf("failed to delete container %s from pod %s: %w", request.ID, podID, err)
	}

	delete(s.containerPodMapping, request.ID)

	return resp, nil
}

// pidsInternal returns the list of process IDs running inside the specified container.
func (s *Service) pidsInternal(ctx context.Context, request *task.PidsRequest) (*task.PidsResponse, error) {
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	ctrCtrl, err := s.getContainerController(request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for pids request: %w", err)
	}

	pids, err := ctrCtrl.Pids(ctx)
	if err != nil {
		err = enrichNotFoundError(err)
		return nil, fmt.Errorf("failed to get pids for container %s: %w", request.ID, err)
	}

	return &task.PidsResponse{
		Processes: pids,
	}, nil
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

// killInternal sends a signal to a process or, when All is set, to every process in the pod.
func (s *Service) killInternal(ctx context.Context, request *task.KillRequest) (*emptypb.Empty, error) {
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	ctrCtrl, err := s.getContainerController(request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for kill request: %w", err)
	}

	// If "all" is set and this is a sandbox (pod) container, collect all
	// workload containers so we can fan out the kill to the entire pod.
	var workloadContainers map[string]*container.Controller
	if request.All {
		if podCtrl, ok := s.getPodController(request.ID); ok {
			workloadContainers = podCtrl.ListContainers()
			// Exclude the sandbox container — it is killed below.
			delete(workloadContainers, request.ID)
		}
	}

	// Fan out kill to all workload containers and the target container concurrently.
	killGroup := errgroup.Group{}
	for _, workloadCtr := range workloadContainers {
		killGroup.Go(func() error {
			return workloadCtr.KillProcess(ctx, request.ExecID, request.Signal, request.All)
		})
	}
	// Target container.
	killGroup.Go(func() error {
		return ctrCtrl.KillProcess(ctx, request.ExecID, request.Signal, request.All)
	})

	// Wait for all kill to complete.
	if err = killGroup.Wait(); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// execInternal creates a new exec process inside the specified container.
func (s *Service) execInternal(ctx context.Context, request *task.ExecProcessRequest) (*emptypb.Empty, error) {
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	var spec specs.Process
	if err := json.Unmarshal(request.Spec.Value, &spec); err != nil {
		return nil, fmt.Errorf("unmarshal process spec: %w", err)
	}

	ctrCtrl, err := s.getContainerController(request.ID)
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

	// Publish the TaskExecAdded event to notify containerd that a new exec has been created.
	s.send(&eventstypes.TaskExecAdded{
		ContainerID: request.ID,
		ExecID:      request.ExecID,
	})

	return &emptypb.Empty{}, nil
}

// resizePtyInternal resizes the pseudo-terminal for the specified process.
func (s *Service) resizePtyInternal(ctx context.Context, request *task.ResizePtyRequest) (*emptypb.Empty, error) {
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	ctrCtrl, err := s.getContainerController(request.ID)
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
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	ctrCtrl, err := s.getContainerController(request.ID)
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

// updateInternal applies a resource update to a pod VM or an individual container.
func (s *Service) updateInternal(ctx context.Context, request *task.UpdateTaskRequest) (*emptypb.Empty, error) {
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	if request.Resources == nil {
		return nil, fmt.Errorf("update container %s: resources cannot be empty: %w", request.ID, errdefs.ErrInvalidArgument)
	}

	resources, err := typeurl.UnmarshalAny(request.Resources)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal resources for container %s update request: %w", request.ID, err)
	}

	// Check if the ID in request matches any podID in podController map.
	// If so, this is a pod-level update — call the appropriate VM controller API.
	if _, ok := s.getPodController(request.ID); ok {
		if err := s.updateVMResources(ctx, resources, request.Annotations); err != nil {
			return nil, fmt.Errorf("failed to update VM resources for pod %s: %w", request.ID, err)
		}
		return &emptypb.Empty{}, nil
	}

	// Otherwise, find the container controller and call Update on it.
	ctrCtrl, err := s.getContainerController(request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update resources for container %s: %w", request.ID, err)
	}

	if err := ctrCtrl.Update(ctx, resources); err != nil {
		return nil, fmt.Errorf("failed to update resources for container %s: %w", request.ID, err)
	}

	return &emptypb.Empty{}, nil
}

// updateVMResources dispatches resource updates to the appropriate VM controller APIs.
func (s *Service) updateVMResources(ctx context.Context, resources interface{}, annots map[string]string) error {
	switch res := resources.(type) {
	case *ctrdtaskapi.PolicyFragment:
		return s.vmController.UpdatePolicyFragment(ctx, guestresource.SecurityPolicyFragment{
			Fragment: res.Fragment,
		})
	case *specs.LinuxResources:
		// Update memory if specified.
		if res.Memory != nil && res.Memory.Limit != nil {
			requestedSizeInMB := uint64(*res.Memory.Limit) / memory.MiB
			if err := s.vmController.UpdateMemory(ctx, requestedSizeInMB); err != nil {
				return fmt.Errorf("failed to update vm memory: %w", err)
			}
		}

		// Translate OCI CPU knobs to HCS processor limits and update if specified.
		if res.CPU != nil {
			processorLimits := &hcsschema.ProcessorLimits{}
			if res.CPU.Quota != nil {
				processorLimits.Limit = uint64(*res.CPU.Quota)
			}
			if res.CPU.Shares != nil {
				processorLimits.Weight = uint64(*res.CPU.Shares)
			}
			if err := s.vmController.UpdateCPU(ctx, processorLimits); err != nil {
				return fmt.Errorf("failed to update vm cpu limits: %w", err)
			}
		}

		// Update CPU group membership if the corresponding annotation is present.
		if cpuGroupID, ok := annots[annotations.CPUGroupID]; ok {
			if err := s.vmController.UpdateCPUGroup(ctx, cpuGroupID); err != nil {
				return fmt.Errorf("failed to update vm cpu group: %w", err)
			}
		}

		return nil
	default:
		return fmt.Errorf("unsupported resource type %T: %w", resources, errdefs.ErrInvalidArgument)
	}
}

// waitInternal blocks until the specified process exits and returns its exit status.
func (s *Service) waitInternal(ctx context.Context, request *task.WaitRequest) (*task.WaitResponse, error) {
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	ctrCtrl, err := s.getContainerController(request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for wait request: %w", err)
	}

	// An empty ExecID means the caller is waiting for the container itself
	// (i.e. the init process + full teardown). Wait on the container
	// controller, which blocks until the container reaches StateTerminated
	// and has finished the teardown.
	if request.ExecID == "" {
		ctrCtrl.Wait(ctx)
	}

	// Get the process controller associated with the ExecID.
	proc, err := ctrCtrl.GetProcess(request.ExecID)
	if err != nil {
		return nil, fmt.Errorf("failed to get process (execID=%q) in container %s: %w", request.ExecID, request.ID, err)
	}

	// Call Wait on the process controller itself.
	proc.Wait(ctx)

	// Get the Process status.
	status := proc.Status(true)

	return &task.WaitResponse{
		ExitStatus: status.ExitStatus,
		ExitedAt:   status.ExitedAt,
	}, nil
}

// statsInternal returns resource usage statistics for the specified container and, for pods, the VM.
func (s *Service) statsInternal(ctx context.Context, request *task.StatsRequest) (*task.StatsResponse, error) {
	if err := s.ensureVMRunning(); err != nil {
		return nil, err
	}

	ctrCtrl, err := s.getContainerController(request.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find container for stats request: %w", err)
	}

	// Get the stats for the requested container.
	ctrStats, err := ctrCtrl.Stats(ctx)
	if err != nil {
		err = enrichNotFoundError(err)
		return nil, fmt.Errorf("failed to get container stats for %s: %w", request.ID, err)
	}

	// Fetch and attach VM stats only for pod-level requests.
	if _, ok := s.getPodController(request.ID); ok {
		vmStats, err := s.vmController.Stats(ctx)
		if err != nil {
			err = enrichNotFoundError(err)
			return nil, fmt.Errorf("failed to get VM stats: %w", err)
		}
		ctrStats.VM = vmStats
	}

	// Marshal the stats into an Any for the response.
	anyStats, err := typeurl.MarshalAny(ctrStats)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal stats: %w", err)
	}

	return &task.StatsResponse{
		Stats: typeurl.MarshalProto(anyStats),
	}, nil
}

// shutdownInternal is a no-op; shim teardown is deferred to SandboxService.ShutdownSandbox.
func (s *Service) shutdownInternal(ctx context.Context, request *task.ShutdownRequest) (*emptypb.Empty, error) {
	// Because this shim strictly implements the Sandbox API,
	// the TaskService no longer has the authority to shut down the shim process.
	// Shim teardown is completely deferred to SandboxService.ShutdownSandbox.

	// Simply log the call for debugging purposes and return.
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
