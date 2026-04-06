//go:build windows

package guestmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/gcs"
)

// Capabilities returns the capabilities of the guest connection.
func (gm *Guest) Capabilities() gcs.GuestDefinedCapabilities {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	return gm.gc.Capabilities()
}

// CreateContainer creates a container in the guest with the given ID and config.
func (gm *Guest) CreateContainer(ctx context.Context, cid string, config interface{}) (*gcs.Container, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	c, err := gm.gc.CreateContainer(ctx, cid, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create container %s: %w", cid, err)
	}

	return c, nil
}

// DumpStacks requests a stack dump from the guest and returns it as a string.
func (gm *Guest) DumpStacks(ctx context.Context) (string, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	dump, err := gm.gc.DumpStacks(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to dump stacks: %w", err)
	}

	return dump, nil
}

// DeleteContainerState removes persisted state for the container identified by cid from the guest.
func (gm *Guest) DeleteContainerState(ctx context.Context, cid string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	err := gm.gc.DeleteContainerState(ctx, cid)
	if err != nil {
		return fmt.Errorf("failed to delete container state for container %s: %w", cid, err)
	}

	return nil
}

// ExecIntoUVM executes commands specified in the requests in the utility VM.
func (gm *Guest) ExecIntoUVM(ctx context.Context, request *cmd.CmdProcessRequest) (int, error) {
	return cmd.ExecInUvm(ctx, gm.gc, request)
}
