//go:build windows && wcow

package vm

import (
	"context"
	"fmt"
	"sync"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/netutil"
	"golang.org/x/sync/errgroup"
)

// setupEntropyListener sets up entropy for WCOW (Windows Containers on Windows) VMs.
//
// For WCOW, entropy setup is not required. Windows VMs have their own internal
// random number generation that does not depend on host-provided entropy.
// This is a no-op implementation to satisfy the platform-specific interface.
//
// For comparison, LCOW VMs require entropy to be provided during boot.
func (c *Manager) setupEntropyListener(_ context.Context, _ *errgroup.Group) {}

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
func (c *Manager) setupLoggingListener(ctx context.Context, _ *errgroup.Group) {
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
			logrus.WithError(err).Fatal("failed to listen for windows logging connections")
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
		if _, ok := <-c.logOutputDone; ok {
			close(c.logOutputDone)
		}
	}()
}

// finalizeGCSConnection finalizes the GCS connection for WCOW UVMs.
// This is called after CreateConnection succeeds and before the VM is considered fully started.
func (c *Manager) finalizeGCSConnection(ctx context.Context) error {
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
