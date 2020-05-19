package main

import (
	"context"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/internal/uvm"
	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/runtime"
	"github.com/containerd/containerd/runtime/v2/task"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

// newWcowPodSandboxTask creates a fake WCOW task with a fake WCOW `init`
// process as a performance optimization rather than creating an actual
// container and process since it is not needed to hold open any namespaces like
// the equivalent on Linux.
//
// It is assumed that this is the only fake WCOW task and that this task owns
// `parent`. When the fake WCOW `init` process exits via `Signal` `parent` will
// be forcibly closed by this task.
func newWcowPodSandboxTask(ctx context.Context, events publisher, id, bundle string, parent *uvm.UtilityVM) shimTask {
	log.G(ctx).WithField("tid", id).Debug("newWcowPodSandboxTask")

	wpst := &wcowPodSandboxTask{
		events: events,
		id:     id,
		init:   newWcowPodSandboxExec(ctx, events, id, bundle),
		host:   parent,
		closed: make(chan struct{}),
	}
	if parent != nil {
		// We have (and own) a parent UVM. Listen for its exit and forcibly
		// close this task. This is not expected but in the event of a UVM crash
		// we need to handle this case.
		go wpst.waitParentExit()
	}
	// In the normal case the `Signal` call from the caller killed this fake
	// init process.
	go wpst.waitInitExit()
	return wpst
}

var _ = (shimTask)(&wcowPodSandboxTask{})

// wcowPodSandboxTask is a special task type that actually holds no real
// resources due to various design differences between Linux/Windows.
//
// For more information on why we can have this stub and in what invariant cases
// it makes sense please see `wcowPodExec`.
//
// Note: If this is a Hypervisor Isolated WCOW sandbox then we do actually track
// the lifetime of the UVM for a WCOW POD but the UVM will have no WCOW
// container/exec init representing the actual POD Sandbox task.
type wcowPodSandboxTask struct {
	events publisher
	// id is the id of this task when it is created.
	//
	// It MUST be treated as read only in the liftetime of the task.
	id string
	// init is the init process of the container.
	//
	// Note: the invariant `container state == init.State()` MUST be true. IE:
	// if the init process exits the container as a whole and all exec's MUST
	// exit.
	//
	// It MUST be treated as read only in the lifetime of the task.
	init *wcowPodSandboxExec
	// host is the hosting VM for this task if hypervisor isolated. If
	// `host==nil` this is an Argon task so no UVM cleanup is required.
	host *uvm.UtilityVM

	closed    chan struct{}
	closeOnce sync.Once
}

func (wpst *wcowPodSandboxTask) ID() string {
	return wpst.id
}

func (wpst *wcowPodSandboxTask) CreateExec(ctx context.Context, req *task.ExecProcessRequest, s *specs.Process) error {
	return errors.Wrap(errdefs.ErrNotImplemented, "WCOW Pod task should never issue exec")
}

func (wpst *wcowPodSandboxTask) GetExec(eid string) (shimExec, error) {
	if eid == "" {
		return wpst.init, nil
	}
	// Cannot exec in an a WCOW sandbox container so all non-init calls fail here.
	return nil, errors.Wrapf(errdefs.ErrNotFound, "exec: '%s' in task: '%s' not found", eid, wpst.id)
}

func (wpst *wcowPodSandboxTask) KillExec(ctx context.Context, eid string, signal uint32, all bool) error {
	e, err := wpst.GetExec(eid)
	if err != nil {
		return err
	}
	if all && eid != "" {
		return errors.Wrapf(errdefs.ErrFailedPrecondition, "cannot signal all for non-empty exec: '%s'", eid)
	}
	err = e.Kill(ctx, signal)
	if err != nil {
		return err
	}
	return nil
}

func (wpst *wcowPodSandboxTask) DeleteExec(ctx context.Context, eid string) (int, uint32, time.Time, error) {
	e, err := wpst.GetExec(eid)
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	switch state := e.State(); state {
	case shimExecStateCreated:
		e.ForceExit(ctx, 0)
	case shimExecStateRunning:
		return 0, 0, time.Time{}, newExecInvalidStateError(wpst.id, eid, state, "delete")
	}
	status := e.Status()

	// Publish the deleted event
	wpst.events.publishEvent(
		ctx,
		runtime.TaskDeleteEventTopic,
		&eventstypes.TaskDelete{
			ContainerID: wpst.id,
			ID:          eid,
			Pid:         status.Pid,
			ExitStatus:  status.ExitStatus,
			ExitedAt:    status.ExitedAt,
		})

	return int(status.Pid), status.ExitStatus, status.ExitedAt, nil
}

func (wpst *wcowPodSandboxTask) Pids(ctx context.Context) ([]options.ProcessDetails, error) {
	return []options.ProcessDetails{
		{
			ProcessID: uint32(wpst.init.Pid()),
			ExecID:    wpst.init.ID(),
		},
	}, nil
}

func (wpst *wcowPodSandboxTask) Wait() *task.StateResponse {
	<-wpst.closed
	return wpst.init.Wait()
}

// close safely closes the hosting UVM. Because of the specialty of this task it
// is assumed that this is always the owner of `wpst.host`. Once closed and all
// resources released it events the `runtime.TaskExitEventTopic` for all
// upstream listeners.
//
// This call is idempotent and safe to call multiple times.
func (wpst *wcowPodSandboxTask) close(ctx context.Context) {
	wpst.closeOnce.Do(func() {
		log.G(ctx).Debug("wcowPodSandboxTask::closeOnce")

		if wpst.host != nil {
			if err := wpst.host.Close(); err != nil {
				log.G(ctx).WithError(err).Error("failed host vm shutdown")
			}
		}
		// Send the `init` exec exit notification always.
		exit := wpst.init.Status()
		wpst.events.publishEvent(
			ctx,
			runtime.TaskExitEventTopic,
			&eventstypes.TaskExit{
				ContainerID: wpst.id,
				ID:          exit.ID,
				Pid:         uint32(exit.Pid),
				ExitStatus:  exit.ExitStatus,
				ExitedAt:    exit.ExitedAt,
			})
		close(wpst.closed)
	})
}

func (wpst *wcowPodSandboxTask) waitInitExit() {
	ctx, span := trace.StartSpan(context.Background(), "wcowPodSandboxTask::waitInitExit")
	defer span.End()
	span.AddAttributes(trace.StringAttribute("tid", wpst.id))

	// Wait for it to exit on its own
	wpst.init.Wait()

	// Close the host and event the exit
	wpst.close(ctx)
}

func (wpst *wcowPodSandboxTask) waitParentExit() {
	ctx, span := trace.StartSpan(context.Background(), "wcowPodSandboxTask::waitParentExit")
	defer span.End()
	span.AddAttributes(trace.StringAttribute("tid", wpst.id))

	werr := wpst.host.Wait()
	if werr != nil {
		log.G(ctx).WithError(werr).Error("parent wait failed")
	}
	// The UVM came down. Force transition the init task (if it wasn't
	// already) to unblock any waiters since the platform wont send any
	// events for this fake process.
	wpst.init.ForceExit(ctx, 1)

	// Close the host and event the exit.
	wpst.close(ctx)
}

func (wpst *wcowPodSandboxTask) ExecInHost(ctx context.Context, req *shimdiag.ExecProcessRequest) (int, error) {
	if wpst.host == nil {
		return 0, errors.New("task is not isolated")
	}
	return hcsoci.ExecInUvm(ctx, wpst.host, req)
}

func (wpst *wcowPodSandboxTask) DumpGuestStacks(ctx context.Context) string {
	if wpst.host != nil {
		stacks, err := wpst.host.DumpStacks(ctx)
		if err != nil {
			log.G(ctx).WithError(err).Warn("failed to capture guest stacks")
		} else {
			return stacks
		}
	}
	return ""
}

func (wpst *wcowPodSandboxTask) Share(ctx context.Context, req *shimdiag.ShareRequest) error {
	if wpst.host == nil {
		return errors.New("task is not isolated")
	}
	options := wpst.host.DefaultVSMBOptions(req.ReadOnly)
	_, err := wpst.host.AddVSMB(ctx, req.HostPath, options)
	if err != nil {
		return err
	}
	sharePath, err := wpst.host.GetVSMBUvmPath(ctx, req.HostPath)
	if err != nil {
		return err
	}
	guestReq := guestrequest.GuestRequest{
		ResourceType: guestrequest.ResourceTypeMappedDirectory,
		RequestType:  requesttype.Add,
		Settings: &hcsschema.MappedDirectory{
			HostPath:      sharePath,
			ContainerPath: req.UvmPath,
			ReadOnly:      req.ReadOnly,
		},
	}
	return wpst.host.GuestRequest(ctx, guestReq)
}

func (wpst *wcowPodSandboxTask) Stats(ctx context.Context) (*stats.Statistics, error) {
	vmStats, err := wpst.host.Stats(ctx)
	if err != nil {
		return nil, err
	}
	stats := &stats.Statistics{}
	stats.VM = vmStats
	return stats, nil
}
