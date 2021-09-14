// +build linux

package hcsv2

import (
	"context"
	"sync"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/guest/gcserr"
	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/guest/runtime"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	"github.com/Microsoft/hcsshim/internal/guest/storage"
	"github.com/Microsoft/hcsshim/internal/guest/transport"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/containerd/cgroups"
	v1 "github.com/containerd/cgroups/stats/v1"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

type Container struct {
	id    string
	vsock transport.Transport

	spec      *oci.Spec
	isSandbox bool

	container   runtime.Container
	initProcess *containerProcess

	etL      sync.Mutex
	exitType prot.NotificationType

	processesMutex sync.Mutex
	processes      map[uint32]*containerProcess
}

func (c *Container) Start(ctx context.Context, conSettings stdio.ConnectionSettings) (int, error) {
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
	stdioSet, err := stdio.Connect(c.vsock, conSettings)
	if err != nil {
		return -1, err
	}

	// Add in anything from the containers OCI spec that might not be set on the exec spec due to the nature of LCOW. There's a couple fields that
	// we edit on the OCI spec in the guest itself so they won't be present on the process spec for the exec. The user for the container is one example.
	adjustExecSpec(c.spec.Process, process)

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

// GetProcess returns the Process with the matching 'pid'. If the 'pid' does
// not exit returns error.
func (c *Container) GetProcess(pid uint32) (Process, error) {
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
	err := c.container.Kill(signal)
	if err != nil {
		return err
	}
	c.setExitType(signal)
	return nil
}

func (c *Container) Delete(ctx context.Context) error {
	if c.isSandbox {
		// remove user mounts in sandbox container
		if err := storage.UnmountAllInPath(ctx, getSandboxMountsDir(c.id), true); err != nil {
			log.G(ctx).WithError(err).Error("failed to unmount sandbox mounts")
		}

		// remove hugepages mounts in sandbox container
		if err := storage.UnmountAllInPath(ctx, getSandboxHugePageMountsDir(c.id), true); err != nil {
			log.G(ctx).WithError(err).Error("failed to unmount hugepages mounts")
		}
	}
	return c.container.Delete()
}

func (c *Container) Update(ctx context.Context, resources interface{}) error {
	return c.container.Update(resources)
}

// Wait waits for the container's init process to exit.
func (c *Container) Wait() prot.NotificationType {
	_, span := trace.StartSpan(context.Background(), "opengcs::Container::Wait")
	defer span.End()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

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
	_, span := trace.StartSpan(ctx, "opengcs::Container::GetStats")
	defer span.End()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	cgroupPath := c.spec.Linux.CgroupsPath
	cg, err := cgroups.Load(cgroups.V1, cgroups.StaticPath(cgroupPath))
	if err != nil {
		return nil, errors.Errorf("failed to get container stats for %v: %v", c.id, err)
	}

	return cg.Stat(cgroups.IgnoreNotExist)
}

func (c *Container) modifyContainerConstraints(ctx context.Context, rt prot.ModifyRequestType, cc *prot.ContainerConstraintsV2) (err error) {
	return c.Update(ctx, cc.Linux)
}

// adjustExecSpec adjusts an OCI runtime specs process field provided for an execed process to contain some of the settings set on the containers spec.
// Some of the OCI spec settings are configured in the guest as there's not enough information available on the host to set them, so the spec set for the
// exec may be missing some things that were present in the final version of the containers spec.
func adjustExecSpec(containerSpec *oci.Process, execSpec *oci.Process) {
	// One of the aforementioned settings is the containers user configured for the init process as we can't verify that the user exists in
	// the container image on the host. A consequence of the user example is any exec will run as root instead of the user configured for
	// the container which can be problematic.
	//
	// Currently through cri there's no way to set a user for an exec, so just assigning whatever user the container ran as to the execs spec
	// is fine for that. However, for any other client that would end up here, that won't hold true. For that scenario, there's no easy way to tell if a
	// client supplied root as the user for an exec explicitly or they simply just didn't fill in the spec, as uid 0 and gid 0 will be root and
	// this is the default value for a uint32. To try and please as many camps as possible, if the uid/gid are changed at all and don't match
	// whatever was set on the containers spec then just use whatever was received. If the exec spec is set to 0/0 for uid/gid, then just inherit
	// the containers as we assume it just wasn't filled in.
	if execSpec.User.UID == 0 && execSpec.User.GID == 0 {
		execSpec.User = containerSpec.User
	}
}
