//go:build windows

package guestmanager

import (
	"context"
	"fmt"
	"net"

	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"

	"github.com/Microsoft/go-winio"
	"github.com/sirupsen/logrus"
)

// Guest manages the GCS connection and guest-side operations for a utility VM.
type Guest struct {
	log *logrus.Entry
	// uvm is the utility VM that this GuestManager is managing.
	// We restrict access to just lifetime manager and VMSocket manager.
	// Other APIs are outside the purview of this package.
	uvm interface {
		vmmanager.LifetimeManager
		vmmanager.VMSocketManager
	}

	gcListener net.Listener         // The listener for the GCS connection
	gc         *gcs.GuestConnection // The GCS connection
}

// New creates a new Guest Manager.
func New(ctx context.Context, uvm interface {
	vmmanager.LifetimeManager
	vmmanager.VMSocketManager
}) (*Guest, error) {
	gm := &Guest{
		log: log.G(ctx).WithField(logfields.UVMID, uvm.ID()),
		uvm: uvm,
	}

	conn, err := winio.ListenHvsock(&winio.HvsockAddr{
		VMID:      uvm.RuntimeID(),
		ServiceID: winio.VsockServiceID(prot.LinuxGcsVsockPort),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to listen for guest connection: %w", err)
	}
	gm.gcListener = conn

	return gm, nil
}

// ConfigOption defines a function that modifies the GCS connection config.
type ConfigOption func(*gcs.GuestConnectionConfig) error

// WithInitializationState applies initial guest state to the GCS connection config.
func WithInitializationState(state *gcs.InitialGuestState) ConfigOption {
	return func(cfg *gcs.GuestConnectionConfig) error {
		cfg.InitGuestState = state
		return nil
	}
}

// CreateConnection accepts the GCS connection and performs initial setup.
func (gm *Guest) CreateConnection(ctx context.Context, opts ...ConfigOption) error {
	// 1. Accept the connection
	conn, err := AcceptConnection(ctx, gm.uvm, gm.gcListener, true)
	if err != nil {
		return fmt.Errorf("failed to connect to GCS: %w", err)
	}
	gm.gcListener = nil // Listener is closed/consumed by AcceptConnection on success.

	// 2. Create the default base configuration
	gcc := &gcs.GuestConnectionConfig{
		Conn:     conn,
		Log:      gm.log, // Ensure gm has a logger field
		IoListen: gcs.HvsockIoListen(gm.uvm.RuntimeID()),
	}

	// 3. Apply all passed options.
	for _, opt := range opts {
		if err := opt(gcc); err != nil {
			return fmt.Errorf("failed to apply GCS config option: %w", err)
		}
	}

	// 4. Start the GCS protocol
	gm.gc, err = gcc.Connect(ctx, true)
	if err != nil {
		return fmt.Errorf("failed to connect to GCS: %w", err)
	}

	// 5. Initial setup required for external GCS connection.
	hvsocketAddress := &hcsschema.HvSocketAddress{
		LocalAddress:  gm.uvm.RuntimeID().String(),
		ParentAddress: prot.WindowsGcsHvHostID.String(),
	}

	err = gm.updateHvSocketAddress(ctx, hvsocketAddress)
	if err != nil {
		return fmt.Errorf("failed to create GCS connection: %w", err)
	}

	return nil
}

// CloseConnection closes any active GCS connection and listener.
func (gm *Guest) CloseConnection() error {
	var firstErr error

	if gm.gc != nil {
		if err := gm.gc.Close(); err != nil {
			firstErr = err
		}
		gm.gc = nil
	}

	if gm.gcListener != nil {
		if err := gm.gcListener.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		gm.gcListener = nil
	}

	return firstErr
}
