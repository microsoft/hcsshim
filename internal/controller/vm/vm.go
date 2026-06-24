//go:build windows && (lcow || wcow)

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
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi"
	"github.com/Microsoft/hcsshim/internal/controller/device/vpci"
	"github.com/Microsoft/hcsshim/internal/hcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/internal/timeout"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	iwin "github.com/Microsoft/hcsshim/internal/windows"

	"github.com/Microsoft/go-winio/pkg/process"
	"github.com/containerd/errdefs"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/windows"
)

// Controller is the VM controller implementation that manages the lifecycle of a Utility VM
// and its associated resources.
type Controller struct {
	vmID  string
	uvm   *vmmanager.UtilityVM
	guest *guestmanager.Guest

	// vmState tracks the current state of the VM lifecycle.
	// Access must be guarded by mu.
	vmState State

	// mu guards the concurrent access to the Controller's fields and operations.
	mu sync.RWMutex

	// logOutputDone is closed when the GCS log output processing goroutine completes.
	logOutputDone chan struct{}

	// Handle to the vmmem process associated with this UVM. Used to look up
	// memory metrics for the UVM.
	vmmemProcess windows.Handle

	// activeExecCount tracks the number of ongoing ExecIntoHost calls.
	activeExecCount atomic.Int64

	// isPhysicallyBacked indicates whether the VM is using physical backing for its memory.
	isPhysicallyBacked bool

	// scsiController manages SCSI devices for this VM.
	scsiController *scsi.Controller

	// vpciController manages virtual PCI device assignments for this VM.
	vpciController *vpci.Controller

	// hcsDocument is the final HCS document used to create this VM,
	// retained for lazy SCSI controller construction and for shipping to
	// the destination during live migration.
	hcsDocument *hcsschema.ComputeSystem

	// compatInfo is the opaque VM compatibility blob rehydrated from
	// a migration snapshot on the destination side.
	compatInfo []byte

	// platformControllers embeds platform-specific sub-controllers (e.g., Plan9 for LCOW).
	platformControllers //nolint:unused,nolintlint // embedded for cross-platform compatibility; empty on WCOW
}

// New creates a new Controller instance in the [StateNotCreated] state.
func New() *Controller {
	return &Controller{
		logOutputDone: make(chan struct{}),
		vmState:       StateNotCreated,
	}
}

// Guest returns the guest manager instance for this VM.
// The guest manager provides access to guest-host communication.
func (c *Controller) Guest() *guestmanager.Guest {
	return c.guest
}

// VM returns the vm manager instance for this VM.
// The vm manager provides access to the VM host side operations.
func (c *Controller) VM() *vmmanager.UtilityVM {
	return c.uvm
}

// State returns the current VM state.
func (c *Controller) State() State {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.vmState
}

// RuntimeID returns the UVM runtime identifier when the VM is created or running.
func (c *Controller) RuntimeID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.vmState != StateCreated && c.vmState != StateRunning {
		return ""
	}

	return c.uvm.RuntimeID().String()
}

// CreateVM creates the VM from either a freshly built HCS document (cold boot)
// or the document imported on the migration destination.
func (c *Controller) CreateVM(ctx context.Context, opts *CreateOptions) error {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "CreateVM"))

	c.mu.Lock()
	defer c.mu.Unlock()

	// Pick the HCS document we hand to vmmanager based on the controller's
	// current state:
	//   - StateNotCreated: cold-boot path; build a fresh document.
	//   - StateMigratingImported: destination side of a live migration; reuse
	//     the document rehydrated by Import and stamp opts.MigrationOptions
	//     onto it.
	// Any other state is invalid for CreateVM.
	var hcsDocument *hcsschema.ComputeSystem
	// Cold boot lands in StateCreated; the destination migration path lands in
	// StateMigratingCreated.
	nextState := StateCreated
	switch c.vmState {
	case StateNotCreated:
		doc, err := c.buildHCSConfig(ctx, opts)
		if err != nil {
			return fmt.Errorf("failed to build VM config: %w", err)
		}
		hcsDocument = doc
	case StateMigratingImported:
		nextState = StateMigratingCreated
		if c.hcsDocument == nil {
			return fmt.Errorf("cannot create VM in state %s: no imported HCS document available", c.vmState)
		}
		if c.hcsDocument.VirtualMachine == nil {
			return fmt.Errorf("cannot create VM in state %s: imported HCS document has no VirtualMachine", c.vmState)
		}
		hcsDocument = c.hcsDocument
		hcsDocument.VirtualMachine.MigrationOptions = opts.MigrationOptions
		if c.compatInfo != nil {
			hcsDocument.VirtualMachine.MigrationOptions.CompatibilityData = &hcsschema.CompatibilityInfo{
				Data: c.compatInfo,
			}
		}
		// SCSI controller is the source of truth for the destination
		// topology (rootfs + hot-added, path-patched); use it verbatim.
		if c.scsiController != nil {
			hcsDocument.VirtualMachine.Devices.Scsi = c.scsiController.HCSAttachments()
		}
	default:
		return fmt.Errorf("cannot create VM: VM is in incorrect state %s", c.vmState)
	}

	// Create the VM via vmmanager.
	uvm, err := vmmanager.Create(ctx, opts.ID, hcsDocument)
	if err != nil {
		return fmt.Errorf("failed to create VM: %w", err)
	}

	// Set the Controller parameters after successful creation.
	c.vmID = opts.ID
	c.uvm = uvm
	// Retain the final HCS document for lazy SCSI init and migration save.
	c.hcsDocument = hcsDocument

	// Initialize the GuestManager for managing guest interactions.
	// We will create the guest connection via GuestManager during StartVM.
	c.guest = guestmanager.New(ctx, uvm)

	// Cold-boot lands in StateCreated; the destination-side migration path
	// lands in StateMigratingCreated, from which Patch and
	// StartWithMigrationOptions drive the controller forward.
	c.vmState = nextState
	return nil
}

// StartVM starts the VM that was previously created via CreateVM.
// It starts the underlying HCS VM, establishes the GCS connection,
// and transitions the VM to [StateRunning].
// On any failure the VM is transitioned to [StateInvalid].
func (c *Controller) StartVM(ctx context.Context, opts *StartOptions) (err error) {
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

	// Set up the host-side hvsock listeners for entropy and logs before
	// starting the VM. The guest dials predefined vsock ports early in boot,
	// so the listeners must be bound up front to avoid a race.
	// Each setup call creates the listener synchronously and dispatches an
	// accept goroutine onto the errgroup:
	//   - entropy: writes seed bytes to the guest, then returns.
	//   - logging: accepts the connection and spawns a long-running relay
	//     for guest logs; the accept goroutine itself returns immediately.
	//
	// We intentionally wait on the error group after VM start but before
	// establishing the GCS connection, to ensure entropy is seeded and the
	// log channel is wired up first.
	if err = c.setupEntropyListener(gctx, g); err != nil {
		return fmt.Errorf("failed to set up entropy listener: %w", err)
	}
	if err = c.setupLoggingListener(gctx, g); err != nil {
		return fmt.Errorf("failed to set up logging listener: %w", err)
	}
	// Open the host-side GCS hvsock listener before VM start so the host
	// is listening when the in-VM GCS dials. Otherwise, GCS falls back to
	// the internal HCS bridge and our accept hangs until timeout.
	if err = c.guest.PrepareConnection(opts.GCSServiceID); err != nil {
		return fmt.Errorf("failed to prepare guest connection: %w", err)
	}

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

	// VM is started, entropy is seeded and log channel is up. Accept the
	// GCS dial on the prepared listener and run the GCS protocol handshake.
	err = c.guest.CreateConnection(ctx, true, opts.ConfigOptions...)
	if err != nil {
		return fmt.Errorf("failed to create guest connection: %w", err)
	}

	err = c.finalizeGCSConnection(ctx)
	if err != nil {
		return fmt.Errorf("failed to finalize GCS connection: %w", err)
	}

	// Set the confidential options if applicable.
	// These are determined from the sandbox options stored during CreateVM.
	confidentialOpts, err := c.buildConfidentialOptions(ctx)
	if err != nil {
		return fmt.Errorf("failed to build confidential options: %w", err)
	}
	if confidentialOpts != nil {
		if err := c.guest.AddSecurityPolicy(ctx, *confidentialOpts); err != nil {
			return fmt.Errorf("failed to set confidential options: %w", err)
		}
	}

	// If all goes well, we can transition the VM to Running state.
	c.vmState = StateRunning

	return nil
}

// UpdatePolicyFragment injects a security policy fragment into the running VM's guest.
func (c *Controller) UpdatePolicyFragment(ctx context.Context, fragment guestresource.SecurityPolicyFragment) error {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "UpdatePolicyFragment"))

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.vmState != StateRunning {
		return fmt.Errorf("cannot update policy fragment: VM is in state %s", c.vmState)
	}

	return c.guest.InjectPolicyFragment(ctx, fragment)
}

// UpdateCPUGroup assigns the VM to the specified CPU group.
func (c *Controller) UpdateCPUGroup(ctx context.Context, cpuGroupID string) error {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "UpdateCPUGroup"))

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.vmState != StateRunning {
		return fmt.Errorf("cannot update cpu group: VM is in state %s", c.vmState)
	}

	if cpuGroupID == "" {
		return errors.New("must specify an ID to use when configuring a VM's cpu group")
	}

	if err := c.uvm.SetCPUGroup(ctx, &hcsschema.CpuGroup{Id: cpuGroupID}); err != nil {
		return fmt.Errorf("failed to set CPU group: %w", err)
	}

	return nil
}

// UpdateCPU updates the CPU limits for the running VM.
func (c *Controller) UpdateCPU(ctx context.Context, limits *hcsschema.ProcessorLimits) error {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "UpdateCPU"))

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.vmState != StateRunning {
		return fmt.Errorf("cannot update cpu limits: VM is in state %s", c.vmState)
	}

	if err := c.uvm.UpdateCPULimits(ctx, limits); err != nil {
		return fmt.Errorf("failed to update vm cpu limits: %w", err)
	}

	return nil
}

// UpdateMemory updates the memory size for the running VM.
// The requestedSizeInMB is normalized before being applied.
func (c *Controller) UpdateMemory(ctx context.Context, requestedSizeInMB uint64) error {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "UpdateMemory"))

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.vmState != StateRunning {
		return fmt.Errorf("cannot update memory: VM is in state %s", c.vmState)
	}

	// Normalize the requested memory size and apply it.
	// Internally, HCS will get the number of pages this corresponds to
	// and attempt to assign pages to numa nodes evenly.
	actual := vmutils.NormalizeMemorySize(ctx, c.vmID, requestedSizeInMB)
	if err := c.uvm.UpdateMemory(ctx, actual); err != nil {
		return fmt.Errorf("failed to update vm memory: %w", err)
	}

	return nil
}

// waitForVMExit blocks until the VM exits and then transitions the VM state to [StateTerminated].
// This is called in StartVM in a background goroutine.
func (c *Controller) waitForVMExit(ctx context.Context) {
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
	}

	// Force the bridge to collapse so in-flight container/process waits are
	// released. CloseConnection is idempotent.
	if c.guest != nil {
		if err := c.guest.CloseConnection(); err != nil {
			log.G(ctx).WithError(err).Warn("close guest connection after vm exit failed")
		}
	}

	c.mu.Unlock()
}

// ExecIntoHost executes a command in the running UVM.
func (c *Controller) ExecIntoHost(ctx context.Context, request *shimdiag.ExecProcessRequest) (int, error) {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "ExecIntoHost"))

	if request.Terminal && request.Stderr != "" {
		return -1, fmt.Errorf("if using terminal, stderr must be empty: %w", errdefs.ErrFailedPrecondition)
	}

	// Validate that the VM is running before allowing exec into it.
	c.mu.RLock()
	if c.vmState != StateRunning {
		c.mu.RUnlock()
		return -1, fmt.Errorf("cannot exec into VM: VM is in incorrect state %s", c.vmState)
	}
	c.mu.RUnlock()

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
func (c *Controller) DumpStacks(ctx context.Context) (string, error) {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "DumpStacks"))

	// Take read lock at this place.
	// The state change cannot happen until we release the lock,
	// so we are sure that the state remains consistent throughout the method.
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Validate that the VM is running before sending dump stacks request to GCS.
	if c.vmState != StateRunning {
		return "", fmt.Errorf("cannot dump stacks: VM is in incorrect state %s", c.vmState)
	}

	if caps := c.guest.Capabilities(); caps != nil && caps.IsDumpStacksSupported() {
		return c.guest.DumpStacks(ctx)
	}

	return "", nil
}

// Wait blocks until the VM exits and all log output processing has completed.
func (c *Controller) Wait(ctx context.Context) error {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "Wait"))

	// Validate that the VM has been created and can be waited on.
	// Terminated VMs can also be waited on where we return immediately.
	c.mu.RLock()
	if c.vmState == StateNotCreated {
		c.mu.RUnlock()
		return fmt.Errorf("cannot wait on VM: VM is in incorrect state %s", c.vmState)
	}

	// Destination terminated before CreateVM: nothing to wait on.
	if c.uvm == nil {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

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
func (c *Controller) Stats(ctx context.Context) (*stats.VirtualMachineStatistics, error) {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "Stats"))

	// Take read lock at this place.
	// The state change cannot happen until we release the lock,
	// so we are sure that the state remains consistent throughout the method.
	c.mu.RLock()
	defer c.mu.RUnlock()

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
func (c *Controller) TerminateVM(ctx context.Context) (err error) {
	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "TerminateVM"))

	c.mu.Lock()
	defer c.mu.Unlock()

	// If the VM has already terminated, we can skip termination and just return.
	// Alternatively, if the VM was never created, we can also skip termination.
	// This makes the TerminateVM operation idempotent.
	if c.vmState == StateTerminated || c.vmState == StateNotCreated {
		return nil
	}

	// Destination migration after Import but before CreateVM: no HCS handle yet.
	if c.uvm == nil {
		c.vmState = StateTerminated
		return nil
	}

	// Best effort attempt to clean up the open vmmem handle.
	_ = windows.Close(c.vmmemProcess)

	// Skip HCS Terminate for a never-started VM (cold-created or destination
	// migration-created). The HCS document sets
	// ShouldTerminateOnLastHandleClosed, so uvm.Close below is sufficient.
	if c.vmState != StateCreated && c.vmState != StateMigratingCreated {
		// Terminate the utility VM. This will also cause the Wait() call in the background goroutine to unblock.
		_ = c.uvm.Terminate(ctx)
	}

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
func (c *Controller) StartTime() (startTime time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if (c.vmState == StateRunning || c.vmState == StateTerminated) && c.uvm != nil {
		return c.uvm.StartedTime()
	}

	return startTime
}

// ExitStatus returns the final status of the VM once it has reached
// [StateTerminated], including the time it stopped and any exit error.
// Returns an error if the VM has not yet stopped.
func (c *Controller) ExitStatus() (*ExitStatus, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.vmState != StateTerminated {
		return nil, fmt.Errorf("cannot get exit status: VM is in incorrect state %s", c.vmState)
	}

	// Destination terminated before CreateVM: no uvm to query.
	if c.uvm == nil {
		return &ExitStatus{}, nil
	}

	// Close-before-Terminate (never-started VM) surfaces ErrAlreadyClosed; treat as clean exit.
	err := c.uvm.ExitError()
	if errors.Is(err, hcs.ErrAlreadyClosed) {
		err = nil
	}

	return &ExitStatus{StoppedTime: c.uvm.StoppedTime(), Err: err}, nil
}
