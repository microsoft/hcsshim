//go:build windows && (lcow || wcow)

package vpci

import (
	"fmt"
)

// Save is not yet supported for the VPCI sub-controller; any tracked state
// indicates a live-migration scenario the controller cannot represent.
func (c *Controller) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.devices) > 0 {
		return fmt.Errorf("vpci controller save not supported: %d devices", len(c.devices))
	}

	return nil
}
