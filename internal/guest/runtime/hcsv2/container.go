//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"syscall"

	cgroups "github.com/containerd/cgroups/v3/cgroup1"
	v1 "github.com/containerd/cgroups/v3/cgroup1/stats"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"

	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/guest/runtime"
	specGuest "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	"github.com/Microsoft/hcsshim/internal/guest/storage"
	"github.com/Microsoft/hcsshim/internal/guest/transport"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// containerStatus has been introduced to enable parallel container creation
type containerStatus uint32

const (
	// containerCreating is the default status set on a Container object, when
	// no underlying runtime container or init process has been assigned
	containerCreating containerStatus = iota
	// containerCreated is the status when a runtime container and init process
	// have been assigned, but runtime start command has not been issued yet
	containerCreated
)

type Container struct {
	id    string
	vsock transport.Transport

	spec          *oci.Spec
	ociBundlePath string
	isSandbox     bool

	container   runtime.Container
	initProcess *containerProcess

	etL      sync.Mutex
	exitType prot.NotificationType

	processesMutex sync.Mutex
	processes      map[uint32]*containerProcess

	// current container (creation) status.
	// Only access through [getStatus] and [setStatus].
	//
	// Note: its more ergonomic to store the uint32 and convert to/from [containerStatus]
	// then use [atomic.Value] and deal with unsafe conversions to/from [any], or use [atomic.Pointer]
	// and deal with the extra pointer dereferencing overhead.
	status atomic.Uint32

	// scratchDirPath represents the path inside the UVM where the scratch directory
	// of this container is located. Usually, this is either `/run/gcs/c/<containerID>` or
	// `/run/gcs/c/<UVMID>/container_<containerID>` if scratch is shared with UVM scratch.
	scratchDirPath string
}

func (c *Container) Start(ctx context.Context, conSettings stdio.ConnectionSettings) (int, error) {
	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::Start")
	stdioSet, err := stdio.Connect(c.vsock, conSettings)
	if err != nil {
		return -1, err
	}
	if c.initProcess.spec.Terminal {
		ttyr := c.container.Tty()
		ttyr.ReplaceConnectionSet(stdioSet)
		ttyr.Start()
	} else {
		pr := c.container.PipeRelay()
		pr.ReplaceConnectionSet(stdioSet)
		pr.CloseUnusedPipes()
		pr.Start()
	}
	err = c.container.Start()
	if err != nil {
		stdioSet.Close()
	}
	return int(c.initProcess.pid), err
}

func (c *Container) ExecProcess(ctx context.Context, process *oci.Process, conSettings stdio.ConnectionSettings) (int, error) {
	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::ExecProcess")
	stdioSet, err := stdio.Connect(c.vsock, conSettings)
	if err != nil {
		return -1, err
	}

	// Add in the core rlimit specified on the container in case there was one set. This makes it so that execed processes can also generate
	// core dumps.
	process.Rlimits = c.spec.Process.Rlimits

	// If the client provided a user for the container to run as, we want to have the exec run as this user as well
	// unless the exec's spec was explicitly set to a different user. If the Username field is filled in on the containers
	// spec, at this point that means the work to find a uid:gid pairing for this username has already been done, so simply
	// assign the uid:gid from the container.
	if process.User.Username != "" {
		// The exec provided a user string of it's own. Grab the uid:gid pairing for the string (if one exists).
		if err := specGuest.SetUserStr(&oci.Spec{Root: c.spec.Root, Process: process}, process.User.Username); err != nil {
			return -1, err
		}
		// Runc doesn't care about this, and just to be safe clear it.
		process.User.Username = ""
	} else if c.spec.Process.User.Username != "" {
		process.User = c.spec.Process.User
	}

	p, err := c.container.ExecProcess(process, stdioSet)
	if err != nil {
		stdioSet.Close()
		return -1, err
	}

	pid := p.Pid()
	c.processesMutex.Lock()
	c.processes[uint32(pid)] = newProcess(c, process, p, uint32(pid), false)
	c.processesMutex.Unlock()
	return pid, nil
}

// InitProcess returns the container's init process
func (c *Container) InitProcess() Process {
	return c.initProcess
}

// GetProcess returns the Process with the matching 'pid'. If the 'pid' does
// not exit returns error.
func (c *Container) GetProcess(pid uint32) (Process, error) {
	//todo: thread a context to this function call
	logrus.WithFields(logrus.Fields{
		logfields.ContainerID: c.id,
		logfields.ProcessID:   pid,
	}).Info("opengcs::Container::GetProcess")
	if c.initProcess.pid == pid {
		return c.initProcess, nil
	}

	c.processesMutex.Lock()
	defer c.processesMutex.Unlock()

	p, ok := c.processes[pid]
	if !ok {
		return nil, gcserr.NewHresultError(gcserr.HrErrNotFound)
	}
	return p, nil
}

// GetAllProcessPids returns all process pids in the container namespace.
func (c *Container) GetAllProcessPids(ctx context.Context) ([]int, error) {
	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::GetAllProcessPids")
	state, err := c.container.GetAllProcesses()
	if err != nil {
		return nil, err
	}
	pids := make([]int, len(state))
	for i, s := range state {
		pids[i] = s.Pid
	}
	return pids, nil
}

// Kill sends 'signal' to the container process.
func (c *Container) Kill(ctx context.Context, signal syscall.Signal) error {
	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::Kill")
	err := c.container.Kill(signal)
	if err != nil {
		return err
	}
	c.setExitType(signal)
	return nil
}

func (c *Container) Delete(ctx context.Context) error {
	entity := log.G(ctx).WithField(logfields.ContainerID, c.id)
	entity.Info("opengcs::Container::Delete")
	if c.isSandbox {
		// remove user mounts in sandbox container
		if err := storage.UnmountAllInPath(ctx, specGuest.SandboxMountsDir(c.id), true); err != nil {
			entity.WithError(err).Error("failed to unmount sandbox mounts")
		}

		// remove hugepages mounts in sandbox container
		if err := storage.UnmountAllInPath(ctx, specGuest.HugePagesMountsDir(c.id), true); err != nil {
			entity.WithError(err).Error("failed to unmount hugepages mounts")
		}
	}

	var retErr error
	if err := c.container.Delete(); err != nil {
		retErr = err
	}

	if err := os.RemoveAll(c.scratchDirPath); err != nil {
		if retErr != nil {
			retErr = fmt.Errorf("errors deleting container state: %w; %w", retErr, err)
		} else {
			retErr = err
		}
	}

	if err := os.RemoveAll(c.ociBundlePath); err != nil {
		if retErr != nil {
			retErr = fmt.Errorf("errors deleting container oci bundle dir: %w; %w", retErr, err)
		} else {
			retErr = err
		}
	}

	return retErr
}

func (c *Container) Update(ctx context.Context, resources interface{}) error {
	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::Update")
	return c.container.Update(resources)
}

// Wait waits for the container's init process to exit.
func (c *Container) Wait() prot.NotificationType {
	_, span := oc.StartSpan(context.Background(), "opengcs::Container::Wait")
	defer span.End()
	span.AddAttributes(trace.StringAttribute(logfields.ContainerID, c.id))

	c.initProcess.writersWg.Wait()
	c.etL.Lock()
	defer c.etL.Unlock()
	return c.exitType
}

// setExitType sets `c.exitType` to the appropriate value based on `signal` if
// `signal` will take down the container.
func (c *Container) setExitType(signal syscall.Signal) {
	c.etL.Lock()
	defer c.etL.Unlock()

	if signal == syscall.SIGTERM {
		c.exitType = prot.NtGracefulExit
	} else if signal == syscall.SIGKILL {
		c.exitType = prot.NtForcedExit
	}
}

// GetStats returns the cgroup metrics for the container.
func (c *Container) GetStats(ctx context.Context) (*v1.Metrics, error) {
	_, span := oc.StartSpan(ctx, "opengcs::Container::GetStats")
	defer span.End()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	cgroupPath := c.spec.Linux.CgroupsPath
	cg, err := cgroups.Load(cgroups.StaticPath(cgroupPath))
	if err != nil {
		return nil, errors.Errorf("failed to get container stats for %v: %v", c.id, err)
	}

	return cg.Stat(cgroups.IgnoreNotExist)
}

func (c *Container) modifyContainerConstraints(ctx context.Context, _ guestrequest.RequestType, cc *guestresource.LCOWContainerConstraints) (err error) {
	return c.Update(ctx, cc.Linux)
}

func (c *Container) getStatus() containerStatus {
	return containerStatus(c.status.Load())
}

func (c *Container) setStatus(st containerStatus) {
	c.status.Store(uint32(st))
}

func (c *Container) ID() string {
	return c.id
}
