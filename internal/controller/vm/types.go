//go:build windows && (lcow || wcow)

package vm

import (
	"time"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"

	"github.com/Microsoft/go-winio/pkg/guid"
)

// CreateOptions contains the configuration needed to create a new VM.
type CreateOptions struct {
	// ID specifies the unique identifier for the VM.
	ID string

	// HCSDocument specifies the HCS schema document used to create the VM.
	HCSDocument *hcsschema.ComputeSystem

	// NoWritableFileShares disallows writable file shares to the UVM.
	NoWritableFileShares bool

	// FullyPhysicallyBacked indicates all memory allocations are backed by physical memory.
	FullyPhysicallyBacked bool
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
