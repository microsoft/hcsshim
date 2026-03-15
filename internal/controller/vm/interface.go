//go:build windows

package vm

import (
	"context"
	"time"

	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"

	"github.com/Microsoft/go-winio/pkg/guid"
)

type Controller interface {
	// Guest returns the guest manager instance for this VM.
	Guest() *guestmanager.Guest

	// State returns the current VM state.
	State() State

	// CreateVM creates and initializes a new VM with the specified options.
	// This prepares the VM but does not start it.
	CreateVM(ctx context.Context, opts *CreateOptions) error

	// StartVM starts the created VM with the specified options.
	// This establishes the guest connection, sets up necessary listeners for
	// guest-host communication, and transitions the VM to StateRunning.
	StartVM(context.Context, *StartOptions) error

	// ExecIntoHost executes a command in the running UVM.
	ExecIntoHost(ctx context.Context, request *shimdiag.ExecProcessRequest) (int, error)

	// DumpStacks dumps the GCS stacks associated with the VM.
	DumpStacks(ctx context.Context) (string, error)

	// Wait blocks until the VM exits or the context is cancelled.
	// It also waits for log output processing to complete.
	Wait(ctx context.Context) error

	Stats(ctx context.Context) (*stats.VirtualMachineStatistics, error)

	TerminateVM(context.Context) error

	// StartTime returns the timestamp when the VM was started.
	// Returns zero value of time.time, if the VM is not in StateRunning or StateTerminated.
	StartTime() time.Time

	// ExitStatus returns information about the stopped VM, including when it
	// stopped and any exit error. Returns an error if the VM is not in StateTerminated.
	ExitStatus() (*ExitStatus, error)
}

// CreateOptions contains the configuration needed to create a new VM.
type CreateOptions struct {
	// ID specifies the unique identifier for the VM.
	ID string

	// HCSDocument specifies the HCS schema document used to create the VM.
	HCSDocument *hcsschema.ComputeSystem
}

// StartOptions contains the configuration needed to start a VM and establish
// the Guest Compute Service (GCS) connection.
type StartOptions struct {
	// GCSServiceID specifies the GUID for the GCS vsock service.
	GCSServiceID guid.GUID

	// ConfigOptions specifies additional configuration options for the guest config.
	ConfigOptions []guestmanager.ConfigOption

	// ConfidentialOptions specifies security policy and confidential computing
	// options for the VM. This is optional and only used for confidential VMs.
	ConfidentialOptions *guestresource.ConfidentialOptions
}

// ExitStatus contains information about a stopped VM's final state.
type ExitStatus struct {
	// StoppedTime is the timestamp when the VM stopped.
	StoppedTime time.Time

	// Err is the error that caused the VM to stop, if any.
	// This will be nil if the VM exited cleanly.
	Err error
}
