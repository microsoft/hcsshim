//go:build windows && wcow

package vpci

import "context"

// waitGuestDeviceReady is a no-op for Windows guests. WCOW does not require a
// guest-side notification as part of vPCI device assignment.
func (c *Controller) waitGuestDeviceReady(_ context.Context, _ string) error {
	return nil
}
