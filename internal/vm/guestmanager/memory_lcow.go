//go:build windows && lcow

package guestmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// UpdateCgroupMemoryLimits refreshes the top-level cgroup memory limits in the guest.
func (gm *Guest) UpdateCgroupMemoryLimits(ctx context.Context) error {
	request := guestrequest.ModificationRequest{
		ResourceType: guestresource.ResourceTypePodCgroupMemoryLimit,
		RequestType:  guestrequest.RequestTypeUpdate,
	}
	if err := gm.modify(ctx, request); err != nil {
		return fmt.Errorf("failed to update cgroup memory limits: %w", err)
	}
	return nil
}
