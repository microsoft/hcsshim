//go:build windows && (lcow || wcow)

package vm

import (
	"time"

	runhcsoptions "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"
	vmsandbox "github.com/Microsoft/hcsshim/sandbox-spec/vm/v2"

	"github.com/Microsoft/go-winio/pkg/guid"
)

// CreateOptions contains the configuration needed to create a new VM.
type CreateOptions struct {
	// ID specifies the unique identifier for the VM.
	ID string

	// Owner specifies the owner name for the VM (e.g., shim name).
	Owner string

	// BundlePath is the path to the bundle directory containing the sandbox config.
	BundlePath string

	// ShimOpts specifies the runtime options for the shim.
	ShimOpts *runhcsoptions.Options

	// SandboxSpec specifies the sandbox specification from CRI.
	SandboxSpec *vmsandbox.Spec
}

// StartOptions contains the configuration needed to start a VM and establish
// the Guest Compute Service (GCS) connection.
type StartOptions struct {
	// GCSServiceID specifies the GUID for the GCS vsock service.
	GCSServiceID guid.GUID

	// ConfigOptions specifies additional configuration options for the guest config.
	ConfigOptions []guestmanager.ConfigOption
}

// ExitStatus contains information about a stopped VM's final state.
type ExitStatus struct {
	// StoppedTime is the timestamp when the VM stopped.
	StoppedTime time.Time

	// Err is the error that caused the VM to stop, if any.
	// This will be nil if the VM exited cleanly.
	Err error
}
