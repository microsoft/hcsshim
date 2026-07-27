//go:build windows && (lcow || wcow)

package vpci

import (
	"fmt"

	"github.com/containerd/errdefs"
)

// Save is a no-op for an empty VPCI sub-controller; it fails if any devices are
// attached, since that state cannot yet be migrated.
func (c *Controller) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.devices) > 0 {
		return fmt.Errorf("vpci controller save not supported: %d devices: %w", len(c.devices), errdefs.ErrFailedPrecondition)
	}

	return nil
}
