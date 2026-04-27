//go:build windows && (lcow || wcow)

package guestmanager

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/sirupsen/logrus"
)

// uvm exposes the subset of [vmmanager.UtilityVM] functionality that the
// guest manager needs.
type uvm interface {
	// ID returns the user-visible identifier for the Utility VM.
	ID() string
	// RuntimeID returns the Hyper-V VM GUID.
	RuntimeID() guid.GUID
	// Wait blocks until the VM exits or ctx is cancelled.
	Wait(ctx context.Context) error
	// ExitError returns the error that caused the VM to exit, if any.
	ExitError() error
}

// Guest manages the GCS connection and guest-side operations for a utility VM.
type Guest struct {
	// mu serializes all operations that interact with the guest connection (gc).
	// This prevents parallel operations on the guest from racing on the GCS connection.
	mu sync.RWMutex

	// gcsServiceID is the GUID that the guest uses to connect to GCS.
	gcsServiceID guid.GUID

	log *logrus.Entry
	// uvm is the utility VM that this GuestManager is managing.
	// We restrict access to just the methods actually needed by this package.
	uvm uvm
	// gc is the active GCS connection to the guest.
	// It will be nil if no connection is active.
	gc *gcs.GuestConnection
	// gcListener is bound by PrepareConnection and consumed by CreateConnection.
	gcListener net.Listener
}

// New creates a new Guest Manager.
func New(ctx context.Context, uvm uvm) *Guest {
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

// PrepareConnection opens the host-side hvsock listener for the given GCS
// service ID. Must be called before VM start so the host is listening when
// the in-VM GCS dials. Idempotent for the same service ID.
func (gm *Guest) PrepareConnection(GCSServiceID guid.GUID) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	// Idempotent if already prepared/connected with the same service ID.
	if gm.gcListener != nil || gm.gc != nil {
		if gm.gcsServiceID != GCSServiceID {
			return fmt.Errorf("gcs service id mismatch: expected %s, got %s", gm.gcsServiceID, GCSServiceID)
		}
		return nil
	}

	l, err := winio.ListenHvsock(&winio.HvsockAddr{
		VMID:      gm.uvm.RuntimeID(),
		ServiceID: GCSServiceID,
	})
	if err != nil {
		return fmt.Errorf("failed to listen for guest connection: %w", err)
	}

	gm.gcsServiceID = GCSServiceID
	gm.gcListener = l
	return nil
}

// CreateConnection accepts the GCS dial on the prepared listener and runs
// the GCS protocol handshake. Must be called after VM start. Idempotent if
// a connection already exists.
func (gm *Guest) CreateConnection(ctx context.Context, opts ...ConfigOption) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if gm.gc != nil {
		return nil
	}
	if gm.gcListener == nil {
		return fmt.Errorf("CreateConnection called before PrepareConnection")
	}

	// AcceptConnection takes ownership of the listener and closes it.
	l := gm.gcListener
	gm.gcListener = nil

	conn, err := vmmanager.AcceptConnection(ctx, gm.uvm, l, true)
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
	gm.mu.Lock()
	defer gm.mu.Unlock()

	var err error

	if gm.gc != nil {
		err = gm.gc.Close()
		gm.gc = nil
	}
	if gm.gcListener != nil {
		_ = gm.gcListener.Close()
		gm.gcListener = nil
	}
	return err
}

// NextPort returns the active GCS connection's IO port allocator
// floor, or 0 if no connection is active. Used by the live-migration
// save path.
func (gm *Guest) NextPort() uint32 {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	if gm.gc == nil {
		return 0
	}
	return gm.gc.NextPort()
}

// SetNextPort raises the active GCS connection's IO port allocator
// floor. No-op if no connection is active. Used by the live-migration
// restore path to skip past vsock ports already in use by restored
// processes.
func (gm *Guest) SetNextPort(p uint32) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if gm.gc == nil {
		return
	}
	gm.gc.SetNextPort(p)
}

// BridgeNextID returns the bridge's next request id, or 0 if no
// connection is active.
func (gm *Guest) BridgeNextID() int64 {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	if gm.gc == nil {
		return 0
	}
	return gm.gc.BridgeNextID()
}

// SeedBridgeNextID raises the bridge's request id allocator floor. No-op
// if no connection is active.
func (gm *Guest) SeedBridgeNextID(next int64) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	if gm.gc == nil {
		return
	}
	gm.gc.SeedBridgeNextID(next)
}
