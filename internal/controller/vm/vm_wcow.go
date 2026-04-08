//go:build windows && wcow

package vm

import (
	"context"
	"fmt"
	"sync"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	"github.com/Microsoft/hcsshim/osversion"

	"github.com/containerd/errdefs"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/netutil"
	"golang.org/x/sync/errgroup"
)

// platformControllers holds platform-specific sub-controllers embedded in [Controller].
// For WCOW, no additional controllers are needed as of now (VSMB will be added later).
type platformControllers struct{}

// setupEntropyListener sets up entropy for WCOW (Windows Containers on Windows) VMs.
//
// For WCOW, entropy setup is not required. Windows VMs have their own internal
// random number generation that does not depend on host-provided entropy.
// This is a no-op implementation to satisfy the platform-specific interface.
//
// For comparison, LCOW VMs require entropy to be provided during boot.
func (c *Controller) setupEntropyListener(_ context.Context, _ *errgroup.Group) {}

// setupLoggingListener sets up logging for WCOW UVMs.
//
// Unlike LCOW, where the log connection must be established before the VM starts,
// WCOW allows the GCS to connect to the logging socket at any time after the VM
// is running. This method sets up a persistent listener that can accept connections
// even if the GCS restarts or reconnects.
//
// The listener is configured to accept only one concurrent connection at a time
// to prevent resource exhaustion, but will accept new connections if the current one is closed.
// This supports scenarios where the logging service inside the VM needs to restart.
func (c *Controller) setupLoggingListener(ctx context.Context, _ *errgroup.Group) {
	// For Windows, the listener can receive a connection later (after VM starts),
	// so we start the output handler in a goroutine with a non-timeout context.
	// This allows the output handler to run independently of the VM creation lifecycle.
	// This is useful for the case when the logging service is restarted.
	go func() {
		baseListener, err := winio.ListenHvsock(&winio.HvsockAddr{
			VMID:      c.uvm.RuntimeID(),
			ServiceID: prot.WindowsLoggingHvsockServiceID,
		})
		if err != nil {
			// Close the output done channel to signal that logging setup
			// has failed and no logs will be processed.
			close(c.logOutputDone)
			logrus.WithError(err).Error("failed to listen for windows logging connections")

			// Return early due to error.
			return
		}

		// Use a WaitGroup to track active log processing goroutines.
		// This ensures we wait for all log processing to complete before closing logOutputDone.
		var wg sync.WaitGroup

		// Limit the listener to accept at most 1 concurrent connection.
		limitedListener := netutil.LimitListener(baseListener, 1)

		for {
			// Accept a connection from the GCS.
			conn, err := vmmanager.AcceptConnection(context.WithoutCancel(ctx), c.uvm, limitedListener, false)
			if err != nil {
				logrus.WithError(err).Error("failed to connect to log socket")
				break
			}

			// Launch a goroutine to process logs from this connection.
			wg.Add(1)
			go func() {
				defer wg.Done()
				logrus.Info("uvm output handler starting")

				// Parse GCS log output and forward it to the host logging system.
				// The parser handles logrus-formatted logs from the GCS.
				vmutils.ParseGCSLogrus(c.uvm.ID())(conn)

				logrus.Info("uvm output handler finished")
			}()
		}

		// Wait for all log processing goroutines to complete.
		wg.Wait()

		// Signal that log output processing has completed.
		close(c.logOutputDone)
	}()
}

// finalizeGCSConnection finalizes the GCS connection for WCOW UVMs.
// This is called after CreateConnection succeeds and before the VM is considered fully started.
func (c *Controller) finalizeGCSConnection(ctx context.Context) error {
	// Prepare the HvSocket address configuration for the external GCS connection.
	// The LocalAddress is the VM's runtime ID, and the ParentAddress is the
	// predefined host ID for Windows GCS communication.
	hvsocketAddress := &hcsschema.HvSocketAddress{
		LocalAddress:  c.uvm.RuntimeID().String(),
		ParentAddress: prot.WindowsGcsHvHostID.String(),
	}

	// Update the guest manager with the HvSocket address configuration.
	// This enables the GCS to establish proper bidirectional communication.
	err := c.guest.UpdateHvSocketAddress(ctx, hvsocketAddress)
	if err != nil {
		return fmt.Errorf("failed to create GCS connection: %w", err)
	}

	return nil
}

// updateVMResources applies Windows VM memory and CPU limits from OCI resources.
func (c *Controller) updateVMResources(ctx context.Context, data interface{}) error {
	resources, ok := data.(*specs.WindowsResources)
	if !ok {
		return fmt.Errorf("invalid resource type %T, expected *specs.WindowsResources", data)
	}

	if resources.Memory != nil {
		if resources.Memory.Limit == nil {
			return fmt.Errorf("invalid windows memory limit: nil")
		}

		sizeInBytes := *resources.Memory.Limit
		// Make a call to the VM's orchestrator to update the VM's size in MB
		// Internally, HCS will get the number of pages this corresponds to and attempt to assign
		// pages to numa nodes evenly
		requestedSizeInMB := sizeInBytes / memory.MiB
		actual := vmutils.NormalizeMemorySize(ctx, c.vmID, requestedSizeInMB)

		if err := c.uvm.UpdateMemory(ctx, actual); err != nil {
			return fmt.Errorf("update vm memory: %w", err)
		}
	}

	// Translate OCI CPU knobs to HCS processor limits.
	if resources.CPU != nil {
		processorLimits := &hcsschema.ProcessorLimits{}
		if resources.CPU.Maximum != nil {
			processorLimits.Limit = uint64(*resources.CPU.Maximum)
		}
		if resources.CPU.Shares != nil {
			processorLimits.Weight = uint64(*resources.CPU.Shares)
		}

		// Support for updating CPU limits was not added until 20H2 build
		if osversion.Get().Build < osversion.V20H2 {
			return errdefs.ErrNotImplemented
		}

		if err := c.uvm.UpdateCPULimits(ctx, processorLimits); err != nil {
			return fmt.Errorf("update vm cpu limits: %w", err)
		}
	}

	return nil
}
