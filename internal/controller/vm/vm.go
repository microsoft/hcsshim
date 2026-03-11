//go:build windows

package vm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	"github.com/Microsoft/hcsshim/internal/cmd"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/internal/timeout"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	iwin "github.com/Microsoft/hcsshim/internal/windows"
	"github.com/containerd/errdefs"

	"github.com/Microsoft/go-winio/pkg/process"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/windows"
)

// Manager is the VM controller implementation that manages the lifecycle of a Utility VM
// and its associated resources.
type Manager struct {
	vmID  string
	uvm   *vmmanager.UtilityVM
	guest *guestmanager.Guest

	// vmState tracks the current state of the VM lifecycle.
	// Access must be guarded by mu.
	vmState State

	// mu guards the concurrent access to the Manager's fields and operations.
	mu sync.Mutex

	// logOutputDone is closed when the GCS log output processing goroutine completes.
	logOutputDone chan struct{}

	// Handle to the vmmem process associated with this UVM. Used to look up
	// memory metrics for the UVM.
	vmmemProcess windows.Handle

	// activeExecCount tracks the number of ongoing ExecIntoHost calls.
	activeExecCount atomic.Int64

	// isPhysicallyBacked indicates whether the VM is using physical backing for its memory.
	isPhysicallyBacked bool
}

// Ensure both the Controller, and it's subset Handle are implemented by Manager.
var _ Controller = (*Manager)(nil)

// NewController creates a new Manager instance in the [StateNotCreated] state.
func NewController() *Manager {
	return &Manager{
		logOutputDone: make(chan struct{}),
		vmState:       StateNotCreated,
	}
}

// Guest returns the guest manager instance for this VM.
// The guest manager provides access to guest-host communication.
func (c *Manager) Guest() *guestmanager.Guest {
	return c.guest
}

// State returns the current VM state.
func (c *Manager) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.vmState
}

// CreateVM creates the VM using the HCS document and initializes device state.
func (c *Manager) CreateVM(ctx context.Context, opts *CreateOptions) error {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "CreateVM"))

	c.mu.Lock()
	defer c.mu.Unlock()

	// In case of duplicate CreateVM call for the same controller, we want to fail.
	if c.vmState != StateNotCreated {
		return fmt.Errorf("cannot create VM: VM is in incorrect state %s", c.vmState)
	}

	// Create the VM via vmmanager.
	uvm, err := vmmanager.Create(ctx, opts.ID, opts.HCSDocument)
	if err != nil {
		return fmt.Errorf("failed to create VM: %w", err)
	}

	// Set the Manager parameters after successful creation.
	c.vmID = opts.ID
	c.uvm = uvm
	// Determine if the VM is physically backed based on the HCS document configuration.
	// We need this while extracting memory metrics, as some of them are only relevant for physically backed VMs.
	c.isPhysicallyBacked = !opts.HCSDocument.VirtualMachine.ComputeTopology.Memory.AllowOvercommit

	// Initialize the GuestManager for managing guest interactions.
	// We will create the guest connection via GuestManager during StartVM.
	c.guest = guestmanager.New(ctx, uvm)

	c.vmState = StateCreated
	return nil
}

// StartVM starts the VM that was previously created via CreateVM.
// It starts the underlying HCS VM, establishes the GCS connection,
// and transitions the VM to [StateRunning].
// On any failure the VM is transitioned to [StateInvalid].
func (c *Manager) StartVM(ctx context.Context, opts *StartOptions) (err error) {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "StartVM"))

	c.mu.Lock()
	defer c.mu.Unlock()

	// If the VM is already running, we can skip the start operation and just return.
	// This makes StartVM idempotent in the case of duplicate calls.
	if c.vmState == StateRunning {
		return nil
	}
	// However, if the VM is in any other state than Created,
	// we should fail as StartVM is only valid on a created VM.
	if c.vmState != StateCreated {
		return fmt.Errorf("cannot start VM: VM is in incorrect state %s", c.vmState)
	}

	defer func() {
		if err != nil {
			// If starting the VM fails, we transition to Invalid state to prevent any further operations on the VM.
			// The VM can be terminated by invoking TerminateVM.
			c.vmState = StateInvalid
		}
	}()

	// save parent context, without timeout to use for wait.
	pCtx := ctx
	// For remaining operations, we expect them to complete within the GCS connection timeout,
	// otherwise we want to fail.
	ctx, cancel := context.WithTimeout(pCtx, timeout.GCSConnectionTimeout)
	log.G(ctx).Debugf("using gcs connection timeout: %s\n", timeout.GCSConnectionTimeout)

	g, gctx := errgroup.WithContext(ctx)
	defer func() {
		_ = g.Wait()
	}()
	defer cancel()

	// we should set up the necessary listeners for guest-host communication.
	// The guest needs to connect to predefined vsock ports.
	// The host must already be listening on these ports before the guest attempts to connect,
	// otherwise the connection would fail.
	c.setupEntropyListener(gctx, g)
	c.setupLoggingListener(gctx, g)

	err = c.uvm.Start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start VM: %w", err)
	}

	// Start waiting on the utility VM in the background.
	// This goroutine will complete when the VM exits.
	go c.waitForVMExit(pCtx)

	// Collect any errors from writing entropy or establishing the log
	// connection.
	if err = g.Wait(); err != nil {
		return err
	}

	err = c.guest.CreateConnection(ctx, opts.GCSServiceID, opts.ConfigOptions...)
	if err != nil {
		return fmt.Errorf("failed to create guest connection: %w", err)
	}

	err = c.finalizeGCSConnection(ctx)
	if err != nil {
		return fmt.Errorf("failed to finalize GCS connection: %w", err)
	}

	// Set the confidential options if applicable.
	if opts.ConfidentialOptions != nil {
		if err := c.guest.AddSecurityPolicy(ctx, *opts.ConfidentialOptions); err != nil {
			return fmt.Errorf("failed to set confidential options: %w", err)
		}
	}

	// If all goes well, we can transition the VM to Running state.
	c.vmState = StateRunning

	return nil
}

// waitForVMExit blocks until the VM exits and then transitions the VM state to [StateTerminated].
// This is called in StartVM in a background goroutine.
func (c *Manager) waitForVMExit(ctx context.Context) {
	// The original context may have timeout or propagate a cancellation
	// copy the original to prevent it affecting the background wait go routine
	ctx = context.WithoutCancel(ctx)
	_ = c.uvm.Wait(ctx)
	// Once the VM has exited, attempt to transition to Terminated.
	// This may be a no-op if TerminateVM already ran concurrently and
	// transitioned the state first — log the discarded error so that
	// concurrent-termination races remain observable.
	c.mu.Lock()
	if c.vmState != StateTerminated {
		c.vmState = StateTerminated
	} else {
		log.G(ctx).WithField("currentState", c.vmState).Debug("waitForVMExit: state transition to Terminated was a no-op")
	}
	c.mu.Unlock()
}

// ExecIntoHost executes a command in the running UVM.
func (c *Manager) ExecIntoHost(ctx context.Context, request *shimdiag.ExecProcessRequest) (int, error) {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "ExecIntoHost"))

	if request.Terminal && request.Stderr != "" {
		return -1, fmt.Errorf("if using terminal, stderr must be empty: %w", errdefs.ErrFailedPrecondition)
	}

	// Validate that the VM is running before allowing exec into it.
	c.mu.Lock()
	if c.vmState != StateRunning {
		c.mu.Unlock()
		return -1, fmt.Errorf("cannot exec into VM: VM is in incorrect state %s", c.vmState)
	}
	c.mu.Unlock()

	// Keep a count of active exec sessions.
	// This will be used to disallow LM with existing exec sessions,
	// as that can lead to orphaned processes within UVM.
	c.activeExecCount.Add(1)
	defer c.activeExecCount.Add(-1)

	cmdReq := &cmd.CmdProcessRequest{
		Args:     request.Args,
		Workdir:  request.Workdir,
		Terminal: request.Terminal,
		Stdin:    request.Stdin,
		Stdout:   request.Stdout,
		Stderr:   request.Stderr,
	}
	return c.guest.ExecIntoUVM(ctx, cmdReq)
}

// DumpStacks dumps the GCS stacks associated with the VM
func (c *Manager) DumpStacks(ctx context.Context) (string, error) {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "DumpStacks"))

	// Validate that the VM is running before sending dump stacks request to GCS.
	c.mu.Lock()
	if c.vmState != StateRunning {
		c.mu.Unlock()
		return "", fmt.Errorf("cannot dump stacks: VM is in incorrect state %s", c.vmState)
	}
	c.mu.Unlock()

	if c.guest.Capabilities().IsDumpStacksSupported() {
		return c.guest.DumpStacks(ctx)
	}

	return "", nil
}

// Wait blocks until the VM exits and all log output processing has completed.
func (c *Manager) Wait(ctx context.Context) error {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "Wait"))

	// Validate that the VM has been created and can be waited on.
	// Terminated VMs can also be waited on where we return immediately.
	c.mu.Lock()
	if c.vmState == StateNotCreated {
		c.mu.Unlock()
		return fmt.Errorf("cannot wait on VM: VM is in incorrect state %s", c.vmState)
	}
	c.mu.Unlock()

	// Wait for the utility VM to exit.
	// This will be unblocked when the VM exits or if the context is cancelled.
	err := c.uvm.Wait(ctx)

	// Wait for the log output processing to complete,
	// which ensures all logs are processed before we return.
	select {
	case <-ctx.Done():
		ctxErr := fmt.Errorf("failed to wait on uvm output processing: %w", ctx.Err())
		err = errors.Join(err, ctxErr)
	case <-c.logOutputDone:
	}

	return err
}

// Stats returns runtime statistics for the VM including processor runtime and
// memory usage. The VM must be in [StateRunning].
func (c *Manager) Stats(ctx context.Context) (*stats.VirtualMachineStatistics, error) {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "Stats"))

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.vmState != StateRunning {
		return nil, fmt.Errorf("cannot get stats: VM is in incorrect state %s", c.vmState)
	}

	// Initialization of vmmemProcess to calculate stats properly for VA-backed UVMs.
	if c.vmmemProcess == 0 {
		vmmemHandle, err := vmutils.LookupVMMEM(ctx, c.uvm.RuntimeID(), &iwin.WinAPI{})
		if err != nil {
			return nil, fmt.Errorf("cannot get stats: %w", err)
		}
		c.vmmemProcess = vmmemHandle
	}

	s := &stats.VirtualMachineStatistics{}
	props, err := c.uvm.PropertiesV2(ctx, hcsschema.PTStatistics, hcsschema.PTMemory)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM properties: %w", err)
	}
	s.Processor = &stats.VirtualMachineProcessorStatistics{}
	s.Processor.TotalRuntimeNS = uint64(props.Statistics.Processor.TotalRuntime100ns * 100)

	s.Memory = &stats.VirtualMachineMemoryStatistics{}
	if !c.isPhysicallyBacked {
		// The HCS properties does not return sufficient information to calculate
		// working set size for a VA-backed UVM. To work around this, we instead
		// locate the vmmem process for the VM, and query that process's working set
		// instead, which will be the working set for the VM.
		memCounters, err := process.GetProcessMemoryInfo(c.vmmemProcess)
		if err != nil {
			return nil, err
		}
		s.Memory.WorkingSetBytes = uint64(memCounters.WorkingSetSize)
	}

	if props.Memory != nil {
		if c.isPhysicallyBacked {
			// If the uvm is physically backed we set the working set to the total amount allocated
			// to the UVM. AssignedMemory returns the number of 4KB pages. Will always be 4KB
			// regardless of what the UVMs actual page size is so we don't need that information.
			s.Memory.WorkingSetBytes = props.Memory.VirtualMachineMemory.AssignedMemory * 4096
		}
		s.Memory.VirtualNodeCount = props.Memory.VirtualNodeCount
		s.Memory.VmMemory = &stats.VirtualMachineMemory{}
		s.Memory.VmMemory.AvailableMemory = props.Memory.VirtualMachineMemory.AvailableMemory
		s.Memory.VmMemory.AvailableMemoryBuffer = props.Memory.VirtualMachineMemory.AvailableMemoryBuffer
		s.Memory.VmMemory.ReservedMemory = props.Memory.VirtualMachineMemory.ReservedMemory
		s.Memory.VmMemory.AssignedMemory = props.Memory.VirtualMachineMemory.AssignedMemory
		s.Memory.VmMemory.SlpActive = props.Memory.VirtualMachineMemory.SlpActive
		s.Memory.VmMemory.BalancingEnabled = props.Memory.VirtualMachineMemory.BalancingEnabled
		s.Memory.VmMemory.DmOperationInProgress = props.Memory.VirtualMachineMemory.DmOperationInProgress
	}
	return s, nil
}

// TerminateVM forcefully terminates a running VM, closes the guest connection,
// and releases HCS resources.
//
// The context is used for all operations, including waits, so timeouts/cancellations may prevent
// proper UVM cleanup.
func (c *Manager) TerminateVM(ctx context.Context) (err error) {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "TerminateVM"))

	c.mu.Lock()
	defer c.mu.Unlock()

	// If the VM has already terminated, we can skip termination and just return.
	// Alternatively, if the VM was never created, we can also skip termination.
	// This makes the TerminateVM operation idempotent.
	if c.vmState == StateTerminated || c.vmState == StateNotCreated {
		return nil
	}

	// Best effort attempt to clean up the open vmmem handle.
	_ = windows.Close(c.vmmemProcess)
	// Terminate the utility VM. This will also cause the Wait() call in the background goroutine to unblock.
	_ = c.uvm.Terminate(ctx)

	if err := c.guest.CloseConnection(); err != nil {
		log.G(ctx).Errorf("close guest connection failed: %s", err)
	}

	err = c.uvm.Close(ctx)
	if err != nil {
		// Transition to Invalid so no further active operations can be performed on the VM.
		c.vmState = StateInvalid
		return fmt.Errorf("failed to close utility VM: %w", err)
	}

	// Set the Terminated status at the end.
	c.vmState = StateTerminated
	return nil
}

// StartTime returns the timestamp when the VM was started.
// Returns zero value of time.Time if the VM has not yet reached
// [StateRunning] or [StateTerminated].
func (c *Manager) StartTime() (startTime time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.vmState == StateRunning || c.vmState == StateTerminated {
		return c.uvm.StartedTime()
	}

	return startTime
}

// ExitStatus returns the final status of the VM once it has reached
// [StateTerminated], including the time it stopped and any exit error.
// Returns an error if the VM has not yet stopped.
func (c *Manager) ExitStatus() (*ExitStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.vmState != StateTerminated {
		return nil, fmt.Errorf("cannot get exit status: VM is in incorrect state %s", c.vmState)
	}

	return &ExitStatus{
		StoppedTime: c.uvm.StoppedTime(),
		Err:         c.uvm.ExitError(),
	}, nil
}
