//go:build windows && lcow

package plan9

import (
	"fmt"
)

// Save is not yet supported for the Plan9 sub-controller; any tracked state
// indicates a live-migration scenario the controller cannot represent.
func (c *Controller) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.sharesByHostPath) > 0 || len(c.reservations) > 0 {
		return fmt.Errorf("plan9 controller save not supported: %d shares, %d reservations", len(c.sharesByHostPath), len(c.reservations))
	}

	return nil
}
