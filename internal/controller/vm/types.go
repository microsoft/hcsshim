//go:build windows && (lcow || wcow)

package vm

import (
	"context"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/process"
	runhcsoptions "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/gcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	vmsandbox "github.com/Microsoft/hcsshim/sandbox-spec/vm/v2"

	"github.com/Microsoft/go-winio/pkg/guid"
)

// Package-level indirection for HCS/winio entry points used by [Controller],
// allowing tests to swap in fakes without standing up a real VM.
var (
	// listenHVSock opens a host-side hvsock listener.
	// The concrete winio.ListenHvsock returns *winio.HvsockListener which
	// satisfies net.Listener. We use net.Listener here so tests can inject fakes.
	listenHVSock = func(addr *winio.HvsockAddr) (net.Listener, error) {
		return winio.ListenHvsock(addr)
	}
	// createVM creates the underlying utility VM via HCS.
	createVM = vmmanager.Create
	// newGuestManager constructs the guest manager for guest-host communication.
	newGuestManager = guestmanager.New
	// lookupVMMEM finds the vmmem process handle for a given VM.
	lookupVMMEM = vmutils.LookupVMMEM
	// getProcessMemoryInfo queries memory stats for a process handle.
	getProcessMemoryInfo = process.GetProcessMemoryInfo
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

// utilityVM is the subset of the underlying [vmmanager.UtilityVM] surface that
// the [Controller] depends on. It is defined as an interface so that the
// Controller can be unit tested without starting up a real HCS VM.
type utilityVM interface {
	ID() string
	RuntimeID() guid.GUID
	Start(ctx context.Context) error
	AcceptConnection(ctx context.Context, l net.Listener, closeConnection bool) (net.Conn, error)
	Wait(ctx context.Context) error
	Terminate(ctx context.Context) error
	Close(ctx context.Context) error
	SetCPUGroup(ctx context.Context, settings *hcsschema.CpuGroup) error
	UpdateCPULimits(ctx context.Context, settings *hcsschema.ProcessorLimits) error
	UpdateMemory(ctx context.Context, memory uint64) error
	PropertiesV2(ctx context.Context, types ...hcsschema.PropertyType) (*hcsschema.Properties, error)
	StartedTime() time.Time
	StoppedTime() time.Time
	ExitError() error
}

// guestManager is the subset of [guestmanager.Guest] that the [Controller]
// depends on. It is defined as an interface so that the Controller can be
// unit tested without a live guest connection.
type guestManager interface {
	PrepareConnection(GCSServiceID guid.GUID) error
	CreateConnection(ctx context.Context, opts ...guestmanager.ConfigOption) error
	CloseConnection() error
	AddSecurityPolicy(ctx context.Context, opts guestresource.ConfidentialOptions) error
	InjectPolicyFragment(ctx context.Context, fragment guestresource.SecurityPolicyFragment) error
	Capabilities() gcs.GuestDefinedCapabilities
	DumpStacks(ctx context.Context) (string, error)
	ExecIntoUVM(ctx context.Context, request *cmd.CmdProcessRequest) (int, error)
}
