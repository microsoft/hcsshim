//go:build windows && lcow

package service

import (
	"context"
	"time"

	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	"github.com/Microsoft/hcsshim/internal/builder/vm/lcow"
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9"
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi"
	"github.com/Microsoft/hcsshim/internal/controller/device/vpci"
	container "github.com/Microsoft/hcsshim/internal/controller/linuxcontainer"
	"github.com/Microsoft/hcsshim/internal/controller/network"
	"github.com/Microsoft/hcsshim/internal/controller/process"
	"github.com/Microsoft/hcsshim/internal/controller/vm"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	hcs "github.com/Microsoft/hcsshim/internal/hcs/v2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"
	"github.com/containerd/containerd/api/runtime/task/v3"
	containerdtypes "github.com/containerd/containerd/api/types/task"
	"github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/protobuf/types/known/anypb"
)

// vmController is the subset of the VM controller that [Service] depends on.
// Implemented by [*vm.Controller].
type vmController interface {
	// State returns the current VM lifecycle state.
	State() vm.State

	// StartTime returns the time when the VM entered the running state.
	StartTime() time.Time

	// ExitStatus returns the final state for a stopped VM.
	ExitStatus() (*vm.ExitStatus, error)

	// SandboxOptions returns the parsed LCOW sandbox options for the VM.
	SandboxOptions() *lcow.SandboxOptions

	// CreateVM builds the HCS document and creates the underlying utility VM.
	CreateVM(ctx context.Context, opts *vm.CreateOptions) error

	// StartVM starts the underlying HCS compute system and establishes the
	// Guest Compute Service (GCS) connection.
	StartVM(ctx context.Context, opts *vm.StartOptions) error

	// TerminateVM forcefully terminates the VM and releases its HCS handle.
	TerminateVM(ctx context.Context) error

	// Wait blocks until the VM exits or ctx is cancelled.
	Wait(ctx context.Context) error

	// Stats returns runtime statistics for the VM.
	Stats(ctx context.Context) (*stats.VirtualMachineStatistics, error)

	// UpdatePolicyFragment injects a security policy fragment into the running guest.
	UpdatePolicyFragment(ctx context.Context, fragment guestresource.SecurityPolicyFragment) error

	// UpdateMemory updates the assigned memory size for the running VM.
	UpdateMemory(ctx context.Context, requestedSizeInMB uint64) error

	// UpdateCPU applies new processor limits to the running VM.
	UpdateCPU(ctx context.Context, limits *hcsschema.ProcessorLimits) error

	// UpdateCPUGroup assigns the VM to the specified CPU group.
	UpdateCPUGroup(ctx context.Context, cpuGroupID string) error

	// ExecIntoHost executes a process inside the utility VM (not inside a container).
	ExecIntoHost(ctx context.Context, request *shimdiag.ExecProcessRequest) (int, error)

	// DumpStacks asks the guest to dump its goroutine stacks.
	DumpStacks(ctx context.Context) (string, error)

	// The methods below are required by [pod.New], which Service hands the
	// vmController to when constructing a [*pod.Controller]. Including them
	// here lets the same field satisfy both interfaces via Go structural typing.

	// RuntimeID returns the unique runtime identifier for the VM.
	RuntimeID() string

	// Guest returns the guest manager used for guest-side operations.
	Guest() *guestmanager.Guest

	// SCSIController returns the SCSI device controller for the VM.
	SCSIController() *scsi.Controller

	// VPCIController returns the vPCI device controller for the VM.
	VPCIController() *vpci.Controller

	// Plan9Controller returns the Plan9 share controller for the VM.
	Plan9Controller() *plan9.Controller

	// NetworkController returns the network controller for the VM.
	NetworkController(networkNamespaceID string) *network.Controller

	// The methods below are required by the migration controller, which the
	// Service hands this vmController to. Declaring them here lets the same
	// field satisfy the migration controller's vmController interface.

	// InitializeLiveMigrationOnSource arms the running source VM for migration.
	InitializeLiveMigrationOnSource(ctx context.Context, options *hcsschema.MigrationInitializeOptions) error

	// Save captures the migrating VM's state into a portable snapshot.
	Save(ctx context.Context) (*anypb.Any, error)

	// Import rehydrates a destination VM controller from a saved snapshot.
	Import(ctx context.Context, env *anypb.Any) error

	// Patch re-ACLs the destination VM's disks after creation.
	Patch(ctx context.Context) error

	// StartLiveMigrationOnSource starts the outgoing migration on the source VM.
	StartLiveMigrationOnSource(ctx context.Context, config *hcs.MigrationConfig) error

	// StartWithMigrationOptions starts the destination VM against the transport.
	StartWithMigrationOptions(ctx context.Context, config *hcs.MigrationConfig) error

	// StartLiveMigrationTransfer begins the memory transfer.
	StartLiveMigrationTransfer(ctx context.Context, options *hcsschema.MigrationTransferOptions) error

	// FinalizeLiveMigration applies the migration's final operation to the VM.
	FinalizeLiveMigration(ctx context.Context, options *hcsschema.MigrationFinalizedOptions) error

	// Resume returns a migrating VM to the running state.
	Resume(ctx context.Context, rebuildBridge bool) error

	// MigrationNotifications returns the VM's migration event stream.
	MigrationNotifications() (<-chan hcsschema.OperationSystemMigrationNotificationInfo, error)
}

// containerController is the subset of the container controller that [Service]
// depends on. Implemented by [*container.Controller] (linuxcontainer).
//
// The child-returning methods (GetProcess, NewProcess) intentionally return the
// concrete [*process.Controller], exactly matching the production controller's
// signatures. This lets [*container.Controller] satisfy the interface directly
// — with no adapter or wrapper — while still allowing tests to substitute a
// mock container controller via [getContainerController].
type containerController interface {
	// Create builds and creates the container in the guest from the OCI spec.
	Create(ctx context.Context, spec *specs.Spec, opts *task.CreateTaskRequest, copts *container.CreateOpts) error

	// Start starts the container's init process and returns its pid.
	Start(ctx context.Context, events chan interface{}) (uint32, error)

	// Wait blocks until the container has terminated and finished teardown.
	Wait(ctx context.Context)

	// Update applies a resource update to the running container.
	Update(ctx context.Context, resources interface{}) error

	// NewProcess creates a new exec process in the container.
	NewProcess(execID string) (*process.Controller, error)

	// GetProcess returns the process controller for the given exec ID.
	GetProcess(execID string) (*process.Controller, error)

	// Pids returns the processes currently running in the container.
	Pids(ctx context.Context) ([]*containerdtypes.ProcessInfo, error)

	// Stats returns resource-usage statistics for the container.
	Stats(ctx context.Context) (*stats.Statistics, error)

	// KillProcess signals a process (or all processes when all is set).
	KillProcess(ctx context.Context, execID string, signal uint32, all bool) error

	// DeleteProcess deletes a process and returns its final state.
	DeleteProcess(ctx context.Context, execID string) (*task.StateResponse, error)
}
