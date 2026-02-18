//go:build windows

package guestmanager

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/pkg/errors"
)

// Manager provides access to guest operations over the GCS connection.
// Call CreateConnection before invoking other methods.
type Manager interface {
	// CreateConnection accepts the GCS connection and performs initial setup.
	CreateConnection(ctx context.Context, opts ...ConfigOption) error
	// CloseConnection closes the GCS connection and listener.
	CloseConnection() error
	// Capabilities returns the guest's declared capabilities.
	Capabilities() gcs.GuestDefinedCapabilities
	// CreateContainer creates a container within guest using ID `cid` and `config`.
	// Once the container is created, it can be managed using the returned `gcs.Container` interface.
	// `gcs.Container` uses the underlying guest connection to issue commands to the guest.
	CreateContainer(ctx context.Context, cid string, config interface{}) (*gcs.Container, error)
	// CreateProcess creates a process in the guest.
	// Once the process is created, it can be managed using the returned `cow.Process` interface.
	// `cow.Process` uses the underlying guest connection to issue commands to the guest.
	CreateProcess(ctx context.Context, settings interface{}) (cow.Process, error)
	// DumpStacks requests a stack dump from the guest and returns it as a string.
	DumpStacks(ctx context.Context) (string, error)
	// DeleteContainerState removes persisted state for the container identified by `cid` from the guest.
	DeleteContainerState(ctx context.Context, cid string) error
}

var _ Manager = (*Guest)(nil)

// Capabilities returns the capabilities of the guest connection.
func (gm *Guest) Capabilities() gcs.GuestDefinedCapabilities {
	return gm.gc.Capabilities()
}

func (gm *Guest) CreateContainer(ctx context.Context, cid string, config interface{}) (*gcs.Container, error) {
	c, err := gm.gc.CreateContainer(ctx, cid, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create container %s", cid)
	}

	return c, nil
}

func (gm *Guest) CreateProcess(ctx context.Context, settings interface{}) (cow.Process, error) {
	p, err := gm.gc.CreateProcess(ctx, settings)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create process")
	}

	return p, nil
}

func (gm *Guest) DumpStacks(ctx context.Context) (string, error) {
	dump, err := gm.gc.DumpStacks(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to dump stacks")
	}

	return dump, nil
}

func (gm *Guest) DeleteContainerState(ctx context.Context, cid string) error {
	err := gm.gc.DeleteContainerState(ctx, cid)
	if err != nil {
		return errors.Wrapf(err, "failed to delete container state for container %s", cid)
	}

	return nil
}
