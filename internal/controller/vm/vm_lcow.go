//go:build windows && lcow

package vm

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"

	"github.com/Microsoft/hcsshim/internal/builder/vm/lcow"
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9"
	"github.com/Microsoft/hcsshim/internal/controller/network"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sync/errgroup"
)

// platformControllers holds platform-specific sub-controllers embedded in [Controller].
// For LCOW, this includes the Plan9 file share controller.
type platformControllers struct {
	// plan9Controller manages Plan9 file share mounts for this VM.
	plan9Controller *plan9.Controller

	// sandboxOptions contains parsed, shim-level configuration for the sandbox.
	sandboxOptions *lcow.SandboxOptions
}

// SandboxOptions returns the sandbox options stored during CreateVM.
func (c *Controller) SandboxOptions() *lcow.SandboxOptions {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.sandboxOptions
}

// buildConfig builds the HCS document for an LCOW VM by calling lcow.BuildSandboxConfig.
// It also stores the sandbox options within the controller.
func (c *Controller) buildHCSConfig(ctx context.Context, opts *CreateOptions) (*hcsschema.ComputeSystem, error) {
	hcsDocument, sandboxOptions, err := lcow.BuildSandboxConfig(ctx, opts.Owner, opts.BundlePath, opts.ShimOpts, opts.SandboxSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sandbox spec: %w", err)
	}

	c.sandboxOptions = sandboxOptions
	c.isPhysicallyBacked = sandboxOptions.FullyPhysicallyBacked

	return hcsDocument, nil
}

// buildConfidentialOptions builds confidential options from the stored sandbox options.
func (c *Controller) buildConfidentialOptions(ctx context.Context) (*guestresource.ConfidentialOptions, error) {
	if c.sandboxOptions == nil || c.sandboxOptions.ConfidentialConfig == nil {
		return nil, nil
	}

	uvmReferenceInfoEncoded, err := vmutils.ParseUVMReferenceInfo(
		ctx,
		vmutils.DefaultLCOWOSBootFilesPath(),
		c.sandboxOptions.ConfidentialConfig.UvmReferenceInfoFile,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to parse UVM reference info: %w", err)
	}

	return &guestresource.ConfidentialOptions{
		EnforcerType:          c.sandboxOptions.ConfidentialConfig.SecurityPolicyEnforcer,
		EncodedSecurityPolicy: c.sandboxOptions.ConfidentialConfig.SecurityPolicy,
		EncodedUVMReference:   uvmReferenceInfoEncoded,
	}, nil
}

// NetworkController returns a new controller for managing network devices on the VM.
// Since we have a namespace per pod, we create a new controller per call.
func (c *Controller) NetworkController(networkNamespaceID string) *network.Controller {
	var policyBasedRouting bool
	if c.sandboxOptions != nil {
		policyBasedRouting = c.sandboxOptions.PolicyBasedRouting
	}

	return network.New(&network.Options{
		NetworkNamespace:   networkNamespaceID,
		PolicyBasedRouting: policyBasedRouting,
	}, c.uvm, c.guest, c.guest)
}

// Plan9Controller returns the singleton controller which can be used
// to manage the Plan9 shares on the Linux UVM.
func (c *Controller) Plan9Controller() *plan9.Controller {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.plan9Controller == nil {
		var noWritableShares bool
		if c.sandboxOptions != nil {
			noWritableShares = c.sandboxOptions.NoWritableFileShares
		}

		c.plan9Controller = plan9.New(c.uvm, c.guest, noWritableShares)
	}

	return c.plan9Controller
}

// setupEntropyListener sets up entropy for LCOW UVMs.
//
// Linux VMs require entropy to initialize their random number generators during boot.
// This method listens on a predefined vsock port and provides cryptographically secure
// random data to the Linux init process when it connects.
func (c *Controller) setupEntropyListener(ctx context.Context, group *errgroup.Group) {
	group.Go(func() error {
		// The Linux guest will connect to this port during init to receive entropy.
		entropyConn, err := winio.ListenHvsock(&winio.HvsockAddr{
			VMID:      c.uvm.RuntimeID(),
			ServiceID: winio.VsockServiceID(vmutils.LinuxEntropyVsockPort),
		})
		if err != nil {
			return fmt.Errorf("failed to listen on hvSocket for entropy: %w", err)
		}

		// Prepare to provide entropy to the init process in the background. This
		// must be done in a goroutine since, when using the internal bridge, the
		// call to Start() will block until the GCS launches, and this cannot occur
		// until the host accepts and closes the entropy connection.
		conn, err := vmmanager.AcceptConnection(ctx, c.uvm, entropyConn, true)
		if err != nil {
			return fmt.Errorf("failed to accept connection on hvSocket for entropy: %w", err)
		}
		defer conn.Close()

		// Write the required amount of entropy to the connection.
		// The init process will read this data and use it to seed the kernel's
		// random number generator (CRNG).
		_, err = io.CopyN(conn, rand.Reader, vmutils.LinuxEntropyBytes)
		if err != nil {
			return fmt.Errorf("failed to write entropy to connection: %w", err)
		}

		return nil
	})
}

// setupLoggingListener sets up logging for LCOW UVMs.
//
// This method establishes a vsock connection to receive log output from GCS
// running inside the Linux VM. The logs are parsed and
// forwarded to the host's logging system for monitoring and debugging.
func (c *Controller) setupLoggingListener(ctx context.Context, group *errgroup.Group) {
	group.Go(func() error {
		// The GCS will connect to this port to stream log output.
		logConn, err := winio.ListenHvsock(&winio.HvsockAddr{
			VMID:      c.uvm.RuntimeID(),
			ServiceID: winio.VsockServiceID(vmutils.LinuxLogVsockPort),
		})
		if err != nil {
			close(c.logOutputDone)
			return fmt.Errorf("failed to listen on hvSocket for logs: %w", err)
		}

		// Accept the connection from the GCS.
		conn, err := vmmanager.AcceptConnection(ctx, c.uvm, logConn, true)
		if err != nil {
			close(c.logOutputDone)
			return fmt.Errorf("failed to accept connection on hvSocket for logs: %w", err)
		}

		// Launch a separate goroutine to process logs for the lifetime of the VM.
		go func() {
			// Parse GCS log output and forward it to the host logging system.
			vmutils.ParseGCSLogrus(c.uvm.ID())(conn)

			// Signal that log output processing has completed.
			// This allows Wait() to ensure all logs are processed before returning.
			close(c.logOutputDone)
		}()

		return nil
	})
}

// finalizeGCSConnection finalizes the GCS connection for LCOW VMs.
// For LCOW, no additional finalization is needed.
func (c *Controller) finalizeGCSConnection(_ context.Context) error {
	return nil
}
