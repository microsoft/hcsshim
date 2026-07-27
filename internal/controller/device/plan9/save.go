//go:build windows && lcow

package plan9

import (
	"fmt"

	"github.com/containerd/errdefs"
)

// Save is a no-op for an empty Plan9 sub-controller; it fails if any shares or
// reservations exist, since that state cannot yet be migrated.
func (c *Controller) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.sharesByHostPath) > 0 || len(c.reservations) > 0 {
		return fmt.Errorf("plan9 controller save not supported: %d shares, %d reservations: %w", len(c.sharesByHostPath), len(c.reservations), errdefs.ErrFailedPrecondition)
	}

	return nil
}
