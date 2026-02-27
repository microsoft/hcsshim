//go:build windows

package guestmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
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
	// gc is the active GCS connection to the guest.
	// It will be nil if no connection is active.
	gc *gcs.GuestConnection
}

// New creates a new Guest Manager.
func New(ctx context.Context, uvm interface {
	vmmanager.LifetimeManager
	vmmanager.VMSocketManager
}) *Guest {
	return &Guest{
		log: log.G(ctx).WithField(logfields.UVMID, uvm.ID()),
		uvm: uvm,
	}
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
func (gm *Guest) CreateConnection(ctx context.Context, GCSServiceID guid.GUID, opts ...ConfigOption) error {
	// The guest needs to connect to predefined GCS port.
	// The host must already be listening on these port before the guest attempts to connect,
	// otherwise the connection would fail.
	vmConn, err := winio.ListenHvsock(&winio.HvsockAddr{
		VMID:      gm.uvm.RuntimeID(),
		ServiceID: GCSServiceID,
	})
	if err != nil {
		return fmt.Errorf("failed to listen for guest connection: %w", err)
	}

	// Accept the connection
	conn, err := vmmanager.AcceptConnection(ctx, gm.uvm, vmConn, true)
	if err != nil {
		return fmt.Errorf("failed to connect to GCS: %w", err)
	}

	// Create the default base configuration
	gcc := &gcs.GuestConnectionConfig{
		Conn:     conn,
		Log:      gm.log, // Ensure gm has a logger field
		IoListen: gcs.HvsockIoListen(gm.uvm.RuntimeID()),
	}

	// Apply all passed options.
	for _, opt := range opts {
		if err := opt(gcc); err != nil {
			return fmt.Errorf("failed to apply GCS config option: %w", err)
		}
	}

	// Start the GCS protocol
	gm.gc, err = gcc.Connect(ctx, true)
	if err != nil {
		return fmt.Errorf("failed to connect to GCS: %w", err)
	}

	return nil
}

// CloseConnection closes any active GCS connection and listener.
func (gm *Guest) CloseConnection() error {
	var err error

	if gm.gc != nil {
		err = gm.gc.Close()
		gm.gc = nil
	}

	return err
}
