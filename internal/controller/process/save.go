//go:build windows && (lcow || wcow)

package process

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/cmd"
	procsave "github.com/Microsoft/hcsshim/internal/controller/process/save"
	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"

	"github.com/containerd/errdefs"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Save captures a running process as a portable payload that a destination
// shim can later restore. It is only valid while the process is running, and on
// success freezes the source until it is resumed or terminated.
func (c *Controller) Save(ctx context.Context) (*anypb.Any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Only a live process has the IO ports and wait id needed to restore it.
	if c.state != StateRunning {
		return nil, fmt.Errorf("process %q in container %q in state %s; want %s", c.execID, c.containerID, c.state, StateRunning)
	}

	// Capture the host-independent identity of the process.
	state := &procsave.Payload{
		SchemaVersion:  procsave.SchemaVersion,
		ExecID:         c.execID,
		Pid:            int32(c.processID),
		Bundle:         c.bundle,
		IoRetryTimeout: durationpb.New(c.ioRetryTimeout),
	}

	// A running process contributes the live IO ports and wait id needed to
	// reattach on the destination.
	if c.process != nil {
		ms := c.process.MigrationState()
		state.StdinPort, state.StdoutPort, state.StderrPort = ms.StdinPort, ms.StdoutPort, ms.StderrPort
		state.WaitCallID = ms.WaitCallID
	}

	// Exec processes carry their OCI spec; init processes leave it unset.
	if c.processSpec != nil {
		raw, err := json.Marshal(c.processSpec)
		if err != nil {
			return nil, fmt.Errorf("marshal process spec for %q/%q: %w", c.containerID, c.execID, err)
		}
		state.OciProcessSpecJson = raw
	}

	// Wrap the encoded payload so the destination can identify and version it.
	payload, err := proto.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal process saved state for %q/%q: %w", c.containerID, c.execID, err)
	}

	// Freeze the source until the migration is resumed or terminated.
	c.state = StateSourceMigrating

	log.G(ctx).WithFields(logrus.Fields{
		logfields.SourceContainerID: c.containerID,
		logfields.ProcessID:         c.processID,
	}).Debug("saved process state")

	return &anypb.Any{TypeUrl: procsave.TypeURL, Value: payload}, nil
}

// Import reconstructs a process from a payload produced by [Controller.Save].
// The result is inert: it holds no live IO or process handle, and operational
// calls are rejected until it has been patched and resumed.
func Import(ctx context.Context, env *anypb.Any, containerID string) (*Controller, error) {
	if env == nil {
		return nil, fmt.Errorf("process saved-state envelope is nil")
	}

	// Refuse envelopes that were not produced by this save format.
	if env.GetTypeUrl() != procsave.TypeURL {
		return nil, fmt.Errorf("unsupported process saved-state type %q", env.GetTypeUrl())
	}

	state := &procsave.Payload{}
	if err := proto.Unmarshal(env.GetValue(), state); err != nil {
		return nil, fmt.Errorf("unmarshal process saved state: %w", err)
	}

	// Reject payloads written by an incompatible shim version.
	if v := state.GetSchemaVersion(); v != procsave.SchemaVersion {
		return nil, fmt.Errorf("unsupported process saved-state schema version %d (want %d)", v, procsave.SchemaVersion)
	}

	// Rebuild the controller in the destination-migrating state, holding the
	// saved IO ports and wait id until resume rebinds them to a live process.
	c := &Controller{
		containerID:    containerID,
		execID:         state.GetExecID(),
		ioRetryTimeout: state.GetIoRetryTimeout().AsDuration(),
		state:          StateDestinationMigrating,
		processID:      int(state.GetPid()),
		bundle:         state.GetBundle(),
		exitedCh:       make(chan struct{}),
		stdinPort:      state.GetStdinPort(),
		stdoutPort:     state.GetStdoutPort(),
		stderrPort:     state.GetStderrPort(),
		waitCallID:     state.GetWaitCallID(),
	}

	// Restore the exec spec when present; absence marks an init process.
	if raw := state.GetOciProcessSpecJson(); len(raw) > 0 {
		spec := &specs.Process{}
		if err := json.Unmarshal(raw, spec); err != nil {
			return nil, fmt.Errorf("unmarshal process spec for %q/%q: %w", c.containerID, c.execID, err)
		}
		c.processSpec = spec
	}

	log.G(ctx).WithFields(logrus.Fields{
		logfields.SourceContainerID: c.containerID,
		logfields.ProcessID:         c.processID,
	}).Debug("imported process state")

	return c, nil
}

// Patch rebinds an imported process to its destination container and opens
// fresh IO ahead of resume. It is valid only on an imported, not-yet-resumed
// process.
func (c *Controller) Patch(ctx context.Context, containerID string, opts *CreateOptions) error {
	if opts == nil {
		return fmt.Errorf("patch options are required: %w", errdefs.ErrInvalidArgument)
	}
	if containerID == "" {
		return fmt.Errorf("destination container id is required: %w", errdefs.ErrInvalidArgument)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateDestinationMigrating {
		return fmt.Errorf("process %q in container %s is in state %s; cannot patch: %w", c.execID, c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	// Reject a terminal/stderr combination that a fresh create would refuse.
	if opts.Terminal && opts.Stderr != "" {
		return fmt.Errorf("process %q in container %s has terminal enabled but stderr is not empty: %w", c.execID, containerID, errdefs.ErrFailedPrecondition)
	}

	// Open IO against the destination first so a failure leaves the process
	// retryable with its old state intact.
	upstreamIO, err := cmd.NewUpstreamIO(ctx, containerID, opts.Stdout, opts.Stderr, opts.Stdin, opts.Terminal, c.ioRetryTimeout)
	if err != nil {
		return fmt.Errorf("create upstream io for process %q in container %s: %w", c.execID, containerID, err)
	}

	// Adopt the destination identity now that IO is secured.
	oldContainerID := c.containerID
	c.containerID = containerID
	c.bundle = opts.Bundle
	c.upstreamIO = upstreamIO

	log.G(ctx).WithFields(logrus.Fields{
		logfields.SourceContainerID:      oldContainerID,
		logfields.DestinationContainerID: containerID,
		logfields.ProcessID:              c.processID,
	}).Debug("patched migrated process IO")

	return nil
}

// Resume returns a migrating process to the running state. On the destination
// it reattaches the patched process to its live guest counterpart, wires up the
// stdio relay, and begins watching for exit. On the source it simply lifts the
// freeze that Save applied, since the live process and IO are still intact.
// Pass events=nil for an init process, whose exit is reported by its owning
// container instead.
func (c *Controller) Resume(ctx context.Context, gcsContainer *gcs.Container, events chan interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Source rollback: the live process and IO are intact, so just lift the
	// freeze that Save applied.
	if c.state == StateSourceMigrating {
		c.state = StateRunning
		return nil
	}

	if c.state != StateDestinationMigrating {
		return fmt.Errorf("process %q in container %q is in state %s; cannot resume: %w", c.execID, c.containerID, c.state, errdefs.ErrFailedPrecondition)
	}

	// Reopen the live process on its preserved IO ports and wait id.
	gcsProc, err := gcsContainer.OpenProcessWithIO(ctx, uint32(c.processID), c.stdinPort, c.stdoutPort, c.stderrPort, c.waitCallID)
	if err != nil {
		return fmt.Errorf("open gcs process pid %d in container %q: %w", c.processID, c.containerID, err)
	}

	// Detach from the caller's context so a canceled RPC does not kill the
	// restored process while IO is being attached.
	execCmd, err := cmd.Attach(context.WithoutCancel(ctx), gcsProc, c.upstreamIO.Stdin(), c.upstreamIO.Stdout(), c.upstreamIO.Stderr())
	if err != nil {
		_ = gcsProc.Close()
		return fmt.Errorf("attach process IO pid %d in container %q: %w", c.processID, c.containerID, err)
	}

	c.hostingSystem = gcsContainer
	c.process = gcsProc
	c.state = StateRunning
	// Ports are single-use; clear them now that IO is reattached.
	c.stdinPort, c.stdoutPort, c.stderrPort = 0, 0, 0

	// Watch for exit in the background, mirroring a freshly started process.
	go c.handleProcessExit(ctx, execCmd, events)

	log.G(ctx).WithField(logfields.ProcessID, c.processID).Debug("resumed migrated process on destination")
	return nil
}

// AbortMigrated terminates an imported, not-yet-resumed process so it can be
// deleted. It is a no-op once the process has been resumed or otherwise left
// the migrating state.
func (c *Controller) AbortMigrated(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateDestinationMigrating {
		return
	}

	log.G(ctx).WithField(logfields.ProcessID, c.processID).Debug("aborting migrated process")
	c.abortInternal(ctx, 137)
}
