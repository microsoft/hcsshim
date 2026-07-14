//go:build windows && wcowprocess

package service

import (
	"context"
	"runtime"

	"github.com/Microsoft/hcsshim/internal/shimdiag"

	"github.com/containerd/errdefs"
)

// diagExecInHostInternal is not implemented; there is no utility VM to exec into.
func (s *Service) diagExecInHostInternal(_ context.Context, _ *shimdiag.ExecProcessRequest) (*shimdiag.ExecProcessResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

// diagTasksInternal returns all containers and their execs in the pod.
func (s *Service) diagTasksInternal(_ context.Context, request *shimdiag.TasksRequest) (*shimdiag.TasksResponse, error) {
	resp := &shimdiag.TasksResponse{}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.pod == nil {
		return resp, nil
	}

	for containerID, ctrCtrl := range s.pod.ListContainers() {
		t := &shimdiag.Task{ID: containerID}
		if request.Execs {
			processes, err := ctrCtrl.ListProcesses()
			if err != nil {
				return nil, err
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

	return resp, nil
}

// diagShareInternal is not implemented; there is no utility VM to share into.
func (s *Service) diagShareInternal(_ context.Context, _ *shimdiag.ShareRequest) (*shimdiag.ShareResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

// diagStacksInternal dumps all host goroutine stacks.
func (s *Service) diagStacksInternal(_ context.Context) (*shimdiag.StacksResponse, error) {
	buf := make([]byte, 4096)
	for {
		buf = buf[:runtime.Stack(buf, true)]
		if len(buf) < cap(buf) {
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	return &shimdiag.StacksResponse{Stacks: string(buf)}, nil
}
