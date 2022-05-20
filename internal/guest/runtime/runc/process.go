//go:build linux
// +build linux

package runc

import (
	"syscall"

	"github.com/Microsoft/hcsshim/internal/guest/runtime"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/sirupsen/logrus"
)

// process represents a process running in a container. It can either be a
// container's init process, or an exec process in a container.
type process struct {
	c         *container
	pid       int
	ttyRelay  *stdio.TtyRelay
	pipeRelay *stdio.PipeRelay
}

var _ runtime.Process = &process{}

func (p *process) Pid() int {
	return p.pid
}

func (p *process) Tty() *stdio.TtyRelay {
	return p.ttyRelay
}

func (p *process) PipeRelay() *stdio.PipeRelay {
	return p.pipeRelay
}

// Delete deletes any state created for the process by either this wrapper or
// runC itself.
func (p *process) Delete() error {
	if err := p.c.r.cleanupProcess(p.c.id, p.pid); err != nil {
		return err
	}
	return nil
}

func (p *process) Wait() (int, error) {
	exitCode, err := p.c.r.waitOnProcess(p.pid)

	l := logrus.WithField(logfields.ContainerID, p.c.id)
	l.WithField(logfields.ContainerID, p.pid).Debug("process wait completed")

	// If the init process for the container has exited, kill everything else in
	// the container. Runc uses the devices cgroup of the container to determine
	// what other processes to kill.
	//
	// We don't issue the kill if the container owns its own pid namespace,
	// because in that case the container kernel will kill everything in the pid
	// namespace automatically (as the container init will be the pid namespace
	// init). This prevents a potential issue where two containers share cgroups
	// but have their own pid namespaces. If we didn't handle this case, runc
	// would kill the processes in both containers when trying to kill
	// either one of them.
	if p == p.c.init && !p.c.ownsPidNamespace {
		// If the init process of a pid namespace terminates, the kernel
		// terminates all other processes in the namespace with SIGKILL. We
		// simulate the same behavior.
		if err := p.c.Kill(syscall.SIGKILL); err != nil {
			l.WithError(err).Error("failed to terminate container after process wait")
		}
	}

	// Wait on the relay to drain any output that was already buffered.
	//
	// At this point, if this is the init process for the container, everything
	// else in the container has been killed, so the write ends of the stdio
	// relay will have been closed.
	//
	// If this is a container exec process instead, then it is possible the
	// relay waits will hang waiting for the write ends to close. This can occur
	// if the exec spawned any child processes that inherited its stdio.
	// Currently we do not do anything to avoid hanging in this case, but in the
	// future we could add special handling.
	if p.ttyRelay != nil {
		p.ttyRelay.Wait()
	}
	if p.pipeRelay != nil {
		p.pipeRelay.Wait()
	}

	l.WithField(logfields.ProcessID, p.pid).Debug("relay wait completed")

	return exitCode, err
}
