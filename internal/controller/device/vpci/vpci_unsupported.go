//go:build windows && !wcow && !lcow

package vpci

import (
	"context"
	"fmt"
)

// waitGuestDeviceReady is a not supported for unsupported guests.
func (c *Controller) waitGuestDeviceReady(_ context.Context, _ string) error {
	return fmt.Errorf("unsupported guest")
}
