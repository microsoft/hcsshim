//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	"github.com/Microsoft/hcsshim/internal/guest/runtime"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

type Process interface {
	// Kill sends `signal` to the process.
	//
	// If the process has already exited returns `gcserr.HrErrNotFound` by contract.
	Kill(ctx context.Context, signal syscall.Signal) error
	// Pid returns the process id of the process.
	Pid() int
	// ResizeConsole resizes the tty to `height`x`width` for the process.
	ResizeConsole(ctx context.Context, height, width uint16) error
	// Wait returns a channel that can be used to wait for the process to exit
	// and gather the exit code. The second channel must be signaled from the
	// caller when the caller has completed its use of this call to Wait.
	Wait() (<-chan int, chan<- bool)
}

// Process is a struct that defines the lifetime and operations associated with
// an oci.Process.
type containerProcess struct {
	// c is the owning container
	c    *Container
	spec *oci.Process
	// cid is the container id that owns this process.
	cid string

	process runtime.Process
	pid     uint32
	// init is `true` if this is the container process itself
	init bool

	// This is only valid post the exitWg
	exitCode int
	// exitWg is marked as done as soon as the underlying
	// (runtime.Process).Wait() call returns, and exitCode has been updated.
	exitWg sync.WaitGroup

	// Used to allow addition/removal to the writersWg after an initial wait has
	// already been issued. It is not safe to call Add/Done without holding this
	// lock.
	writersSyncRoot sync.Mutex
	// Used to track the number of writers that need to finish
	// before the process can be marked for cleanup.
	writersWg sync.WaitGroup
	// Used to track the 1st caller to the writersWg that successfully
	// acknowledges it wrote the exit response.
	writersCalled bool
}

var _ Process = &containerProcess{}

// newProcess returns a containerProcess struct that has been initialized with
// an outstanding wait for process exit, and post exit an outstanding wait for
// process cleanup to release all resources once at least 1 waiter has
// successfully written the exit response.
func newProcess(c *Container, spec *oci.Process, process runtime.Process, pid uint32, init bool) *containerProcess {
	p := &containerProcess{
		c:       c,
		spec:    spec,
		process: process,
		init:    init,
		cid:     c.id,
		pid:     pid,
	}
	p.exitWg.Add(1)
	p.writersWg.Add(1)
	go func() {
		ctx, span := oc.StartSpan(context.Background(), "newProcess::waitBackground")
		defer span.End()
		span.AddAttributes(
			trace.StringAttribute(logfields.ContainerID, p.cid),
			trace.Int64Attribute(logfields.ProcessID, int64(p.pid)))

		// Wait for the process to exit
		exitCode, err := p.process.Wait()
		if err != nil {
			log.G(ctx).WithError(err).Error("failed to wait for runc process")
		}
		p.exitCode = exitCode
		log.G(ctx).WithField("exitCode", p.exitCode).Debug("process exited")

		// Free any process waiters
		p.exitWg.Done()

		// Schedule the removal of this process object from the map once at
		// least one waiter has read the result
		go func() {
			p.writersWg.Wait()
			// cleanup the process state
			if derr := p.process.Delete(); derr != nil {
				log.G(ctx).WithFields(logrus.Fields{
					"cid": p.cid,
					"pid": p.pid,
				}).Debugf("process cleanup error: %s", derr)
			}
			c.processesMutex.Lock()

			_, span := oc.StartSpan(context.Background(), "newProcess::waitBackground::waitAllWaiters")
			defer span.End()
			span.AddAttributes(
				trace.StringAttribute("cid", p.cid),
				trace.Int64Attribute("pid", int64(p.pid)))

			delete(c.processes, p.pid)
			c.processesMutex.Unlock()
		}()
	}()
	return p
}

// Kill sends 'signal' to the process.
//
// If the process has already exited returns `gcserr.HrErrNotFound` by contract.
func (p *containerProcess) Kill(_ context.Context, signal syscall.Signal) error {
	if err := syscall.Kill(int(p.pid), signal); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return gcserr.NewHresultError(gcserr.HrErrNotFound)
		}
		return err
	}

	if p.init {
		p.c.setExitType(signal)
	}

	return nil
}

func (p *containerProcess) Pid() int {
	return int(p.pid)
}

// ResizeConsole resizes the tty to `height`x`width` for the process.
func (p *containerProcess) ResizeConsole(_ context.Context, height, width uint16) error {
	tty := p.process.Tty()
	if tty == nil {
		return fmt.Errorf("pid: %d, is not a tty and cannot be resized", p.pid)
	}
	return tty.ResizeConsole(height, width)
}

// Wait returns a channel that can be used to wait for the process to exit and
// gather the exit code. The second channel must be signaled from the caller
// when the caller has completed its use of this call to Wait.
func (p *containerProcess) Wait() (<-chan int, chan<- bool) {
	ctx, span := oc.StartSpan(context.Background(), "opengcs::containerProcess::Wait")
	span.AddAttributes(
		trace.StringAttribute("cid", p.cid),
		trace.Int64Attribute("pid", int64(p.pid)))

	exitCodeChan := make(chan int, 1)
	doneChan := make(chan bool)

	// Increment our waiters for this waiter
	p.writersSyncRoot.Lock()
	p.writersWg.Add(1)
	p.writersSyncRoot.Unlock()

	go func() {
		bgExitCodeChan := make(chan int, 1)
		go func() {
			p.exitWg.Wait()
			bgExitCodeChan <- p.exitCode
		}()

		// Wait for the exit code or the caller to stop waiting.
		select {
		case exitCode := <-bgExitCodeChan:
			exitCodeChan <- exitCode

			// The caller got the exit code. Wait for them to tell us they have
			// issued the write
			<-doneChan
			p.writersSyncRoot.Lock()
			// Decrement this waiter
			log.G(ctx).Debug("wait completed, releasing wait count")

			p.writersWg.Done()
			if !p.writersCalled {
				// We have at least 1 response for the exit code for this
				// process. Decrement the release waiter that will free the
				// process resources when the writersWg hits 0
				log.G(ctx).Debug("first wait completed, releasing first wait count")

				p.writersCalled = true
				p.writersWg.Done()
			}
			p.writersSyncRoot.Unlock()
			span.End()

		case <-doneChan:
			// In this case the caller timed out before the process exited. Just
			// decrement the waiter but since no exit code we just deal with our
			// waiter.
			p.writersSyncRoot.Lock()
			log.G(ctx).Debug("wait canceled before exit, releasing wait count")

			p.writersWg.Done()
			p.writersSyncRoot.Unlock()
			span.End()
		}
	}()
	return exitCodeChan, doneChan
}

func newExternalProcess(ctx context.Context, cmd *exec.Cmd, tty *stdio.TtyRelay, onRemove func(pid int)) (*externalProcess, error) {
	ep := &externalProcess{
		cmd:       cmd,
		tty:       tty,
		waitBlock: make(chan struct{}),
		remove:    onRemove,
	}
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to call Start for external process")
	}
	if tty != nil {
		tty.Start()
	}
	go func() {
		_ = cmd.Wait()
		ep.exitCode = cmd.ProcessState.ExitCode()
		log.G(ctx).WithFields(logrus.Fields{
			"pid":      cmd.Process.Pid,
			"exitCode": ep.exitCode,
		}).Debug("external process exited")
		if ep.tty != nil {
			ep.tty.Wait()
		}
		close(ep.waitBlock)
	}()
	return ep, nil
}

type externalProcess struct {
	cmd *exec.Cmd
	tty *stdio.TtyRelay

	waitBlock chan struct{}
	exitCode  int

	removeOnce sync.Once
	remove     func(pid int)
}

var _ Process = &externalProcess{}

func (ep *externalProcess) Kill(_ context.Context, signal syscall.Signal) error {
	if err := syscall.Kill(ep.cmd.Process.Pid, signal); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return gcserr.NewHresultError(gcserr.HrErrNotFound)
		}
		return err
	}
	return nil
}

func (ep *externalProcess) Pid() int {
	return ep.cmd.Process.Pid
}

func (ep *externalProcess) ResizeConsole(_ context.Context, height, width uint16) error {
	if ep.tty == nil {
		return fmt.Errorf("pid: %d, is not a tty and cannot be resized", ep.cmd.Process.Pid)
	}
	return ep.tty.ResizeConsole(height, width)
}

func (ep *externalProcess) Wait() (<-chan int, chan<- bool) {
	_, span := oc.StartSpan(context.Background(), "opengcs::externalProcess::Wait")
	span.AddAttributes(trace.Int64Attribute("pid", int64(ep.cmd.Process.Pid)))

	exitCodeChan := make(chan int, 1)
	doneChan := make(chan bool)

	go func() {
		defer close(exitCodeChan)

		// Wait for the exit code or the caller to stop waiting.
		select {
		case <-ep.waitBlock:
			// Process exited send the exit code and wait for caller to close.
			exitCodeChan <- ep.exitCode
			<-doneChan
			// At least one waiter was successful, remove this external process.
			ep.removeOnce.Do(func() {
				ep.remove(ep.cmd.Process.Pid)
			})
		case <-doneChan:
			// Caller closed early, do nothing.
		}
	}()
	return exitCodeChan, doneChan
}
