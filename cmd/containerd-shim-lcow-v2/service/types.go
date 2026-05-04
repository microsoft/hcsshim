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
	"github.com/Microsoft/hcsshim/internal/controller/network"
	"github.com/Microsoft/hcsshim/internal/controller/vm"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"
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
}
