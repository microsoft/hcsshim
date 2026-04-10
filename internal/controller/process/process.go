//go:build windows && (lcow || wcow)

package process

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	eventstypes "github.com/containerd/containerd/api/events"

	"github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Controller manages the lifecycle of a single process (init or exec)
// within a container.
type Controller struct {
	mu sync.RWMutex

	// containerID is the ID of the owning container.
	// This is the client facing containerID.
	containerID string

	// execID is the unique identifier for this exec instance.
	// The init process uses an empty string.
	execID string

	// hostingSystem is the UVM connection that hosts this process.
	hostingSystem cow.ProcessHost

	// ioRetryTimeout is the duration to retry IO connection setup.
	ioRetryTimeout time.Duration

	// state is the current lifecycle state.
	state State

	// process is the underlying OS process handle, returned by cmd.
	process cow.Process

	// processID is the OS-level PID, set after Start.
	processID int

	// upstreamIO holds the upstream IO connections (stdin/stdout/stderr pipes).
	// Set during Create, consumed during Start.
	upstreamIO cmd.UpstreamIO

	// bundle is the path to the OCI bundle directory.
	bundle string

	// processSpec is the OCI process spec for exec processes.
	processSpec *specs.Process

	// exitedAt is the timestamp when the process exited.
	exitedAt time.Time

	// exitCode is the process exit code.
	// Defaults to 255 for processes that have not exited.
	exitCode uint32

	// exitedCh is closed when the process has exited and all cleanup is done.
	exitedCh chan struct{}
}

// New creates a [Controller] for a process in the given container.
func New(containerID string, execID string, hostingSystem cow.ProcessHost, ioRetryTimeout time.Duration) *Controller {
	return &Controller{
		containerID:    containerID,
		execID:         execID,
		hostingSystem:  hostingSystem,
		ioRetryTimeout: ioRetryTimeout,
		state:          StateNotCreated,
		exitCode:       255, // By design for non-exited process status.
		exitedCh:       make(chan struct{}),
	}
}

// Pid returns the OS-level process ID.
func (c *Controller) Pid() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.processID
}

// Create sets up upstream IO connections and stores the process spec,
// transitioning the controller from StateNotCreated to StateCreated.
func (c *Controller) Create(ctx context.Context, opts *CreateOptions) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateNotCreated {
		return fmt.Errorf("process %q in container %s is in state %s; cannot create: %w", c.execID, c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	if opts.Terminal && opts.Stderr != "" {
		return fmt.Errorf("process %q in container %s has terminal enabled but stderr is not empty: %w", c.execID, c.containerID, errdefs.ErrFailedPrecondition)
	}

	// Establish upstream IO connections for stdin, stdout, and stderr.
	upstreamIO, err := cmd.NewUpstreamIO(ctx, c.containerID, opts.Stdout, opts.Stderr, opts.Stdin, opts.Terminal, c.ioRetryTimeout)
	if err != nil {
		return fmt.Errorf("create upstream io for process %q in container %s: %w", c.execID, c.containerID, err)
	}

	c.upstreamIO = upstreamIO
	c.bundle = opts.Bundle
	c.processSpec = opts.Spec
	c.state = StateCreated

	return nil
}

// Start launches the process inside the hosting system and returns the PID.
func (c *Controller) Start(ctx context.Context, events chan interface{}) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateCreated {
		return -1, fmt.Errorf("process %q in container %s is in state %s; cannot start: %w", c.execID, c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	// Build the command to run inside the container.
	// An init exec passes the process as part of the config. We only pass
	// the spec if this is a true exec.
	execCmd := &cmd.Cmd{
		Host:   c.hostingSystem,
		Stdin:  c.upstreamIO.Stdin(),
		Stdout: c.upstreamIO.Stdout(),
		Stderr: c.upstreamIO.Stderr(),
		Log: log.G(ctx).WithFields(logrus.Fields{
			logfields.ContainerID: c.containerID,
			logfields.ExecID:      c.execID,
		}),
		CopyAfterExitTimeout: time.Second,
		Spec:                 c.processSpec,
	}

	// Start the process and abort on failure.
	if err := execCmd.Start(); err != nil {
		c.abortInternal(ctx, 1)
		return -1, err
	}

	// Track the running process and launch background exit handler.
	c.process = execCmd.Process
	c.processID = c.process.Pid()
	c.state = StateRunning

	go c.handleProcessExit(ctx, execCmd, events)

	return c.processID, nil
}

// handleProcessExit blocks until the process exits, cleans up IO, and
// publishes the exit event via events channel.
func (c *Controller) handleProcessExit(ctx context.Context, execCmd *cmd.Cmd, events chan interface{}) {
	// Detach from the caller's context so upstream cancellation does
	// not abort the background teardown.
	ctx = context.WithoutCancel(ctx)

	// Wait for the process to exit, drain all IO copies, and close the
	// underlying process handle.
	if err := execCmd.Wait(); err != nil {
		log.G(ctx).WithError(err).Warn("process exit wait failed")
	}

	exitCode := execCmd.ExitState.ExitCode()

	// Record the exit status under the lock.
	c.mu.Lock()
	if c.state == StateTerminated {
		log.G(ctx).Warnf("process %s is already in terminated state", c.execID)
		c.mu.Unlock()
		return
	}
	c.state = StateTerminated
	c.exitCode = uint32(exitCode)
	c.exitedAt = time.Now()
	c.mu.Unlock()

	// Release upstream IO connections.
	c.upstreamIO.Close(ctx)

	// Unblock any waiters.
	close(c.exitedCh)

	// Publish the exit event after all cleanup is done.
	// We do not publish exit event for init process wherein
	// the event is sent after container teardown is complete.
	if events != nil {
		status := c.Status(true)
		events <- &eventstypes.TaskExit{
			ContainerID: c.containerID,
			ID:          status.ExecID,
			Pid:         status.Pid,
			ExitStatus:  status.ExitStatus,
			ExitedAt:    status.ExitedAt,
		}
	}
}

// Status returns the current containerd-compatible state of the process.
// When isDetailed is true, the response includes bundle, IO paths,
// terminal flag, exit code, and exit timestamp.
func (c *Controller) Status(isDetailed bool) *task.StateResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()

	resp := &task.StateResponse{
		ID:     c.containerID,
		ExecID: c.execID,
		Pid:    uint32(c.processID),
		Status: c.state.ContainerdStatus(),
	}

	if isDetailed && c.state != StateNotCreated {
		resp.Bundle = c.bundle
		resp.Stdin = c.upstreamIO.StdinPath()
		resp.Stdout = c.upstreamIO.StdoutPath()
		resp.Stderr = c.upstreamIO.StderrPath()
		resp.Terminal = c.upstreamIO.Terminal()
		resp.ExitStatus = c.exitCode
		resp.ExitedAt = timestamppb.New(c.exitedAt)
	}

	return resp
}

// State returns the current lifecycle state.
func (c *Controller) State() State {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.state
}

// ResizeConsole resizes the pseudo-TTY of a running process.
func (c *Controller) ResizeConsole(ctx context.Context, width, height uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateRunning {
		return fmt.Errorf("process %q in container %s is in state %s; cannot resize console: %w", c.execID, c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	if !c.upstreamIO.Terminal() {
		return fmt.Errorf("process %q in container %s is not a tty: %w", c.execID, c.containerID, errdefs.ErrFailedPrecondition)
	}

	return c.process.ResizeConsole(ctx, uint16(width), uint16(height))
}

// CloseIO closes the upstream stdin connection, unblocking any pending
// IO copy. This is safe to call multiple times.
func (c *Controller) CloseIO(ctx context.Context) {
	// Taking read lock is sufficient as CloseStdin itself is thread safe.
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.state == StateNotCreated {
		return
	}

	// If we have any upstream IO we close the upstream connection. This will
	// unblock the `io.Copy` in the `cmd.Start()` call which will signal
	// `cmd.CloseStdin()`. This is safe to call multiple times.
	c.upstreamIO.CloseStdin(ctx)
}

// Wait blocks until the process has exited or the context is cancelled.
func (c *Controller) Wait(ctx context.Context) {
	select {
	case <-c.exitedCh:
	case <-ctx.Done():
	}
}

// Kill delivers a signal to the process or terminates it.
//
// signalOptions contains the platform-specific signal options (e.g.,
// SignalProcessOptionsWCOW or SignalProcessOptionsLCOW). The caller is
// responsible for validating the signal and producing the correct options
// for the platform. When signalOptions is nil the terminate path is used.
func (c *Controller) Kill(ctx context.Context, signalOptions interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.state {
	case StateCreated:
		// The process was never started. Transition directly to terminated state.
		c.abortInternal(ctx, 1)
		return nil

	case StateRunning:
		var isDelivered bool
		var err error

		if signalOptions != nil {
			isDelivered, err = c.process.Signal(ctx, signalOptions)
		} else {
			// Legacy path: signals are not supported, issue a direct terminate.
			isDelivered, err = c.process.Kill(ctx)
		}

		if err != nil {
			if hcs.IsAlreadyStopped(err) {
				// Desired state matches actual state — not an error.
				return nil
			}
			return fmt.Errorf("failed to kill process: %w", err)
		}

		if !isDelivered {
			return fmt.Errorf("process %q in container %s was not found: %w", c.execID, c.containerID, errdefs.ErrNotFound)
		}
		return nil

	case StateTerminated:
		// The process already exited — desired state matches actual state.
		return nil

	default:
		return fmt.Errorf("process %q in container %s is in an unexpected state %s: %w", c.execID, c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}
}

// Delete prepares the process for removal from the container's process table.
func (c *Controller) Delete(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.state {
	case StateTerminated:
		// Expected state — the process has exited and is ready for cleanup.
		return nil

	case StateCreated:
		// The process was created but never started. Abort it to release IO
		// resources and unblock any waiters.
		c.abortInternal(ctx, 0)
		return nil

	case StateRunning:
		// A running process must be explicitly killed before it can be deleted.
		return fmt.Errorf("process %q in container %s is still running; cannot delete: %w", c.execID, c.containerID, errdefs.ErrFailedPrecondition)

	default:
		return fmt.Errorf("process %q in container %s is in unexpected state %s for delete: %w", c.execID, c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}
}

// abortInternal performs the abort teardown while the caller already holds c.mu.
func (c *Controller) abortInternal(ctx context.Context, exitCode uint32) {
	// No OS-level process exists — transition directly to terminated state.
	c.state = StateTerminated
	c.exitCode = exitCode
	c.exitedAt = time.Now()

	// Release upstream IO connections that were never used.
	c.upstreamIO.Close(ctx)

	// Unblock any waiters.
	close(c.exitedCh)
}
