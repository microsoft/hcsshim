//go:build windows && !wcow && !lcow

package vm

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
)

// platformControllers holds platform-specific sub-controllers embedded in [Controller].
// For unsupported guests, no additional controllers are needed.
type platformControllers struct{}

// setupEntropyListener is a no-op for unsupported guests.
func (c *Controller) setupEntropyListener(_ context.Context, _ *errgroup.Group) {}

// setupLoggingListener is a no-op for unsupported guests.
func (c *Controller) setupLoggingListener(_ context.Context, _ *errgroup.Group) {}

// finalizeGCSConnection is not supported for unsupported guests.
func (c *Controller) finalizeGCSConnection(_ context.Context) error {
	return fmt.Errorf("unsupported guest")
}

// updateVMResources is not supported for unsupported guests.
func (c *Controller) updateVMResources(_ context.Context, _ interface{}) error {
	return fmt.Errorf("unsupported guest")
}
