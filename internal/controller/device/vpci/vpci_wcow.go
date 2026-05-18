//go:build windows && wcow

package vpci

import (
	"context"

	"github.com/Microsoft/go-winio/pkg/guid"
)

// guestVPCI exposes vPCI device operations in the guest.
// Not applicable for WCOW guests.
type guestVPCI interface{}

// waitGuestDeviceReady is a no-op for Windows guests. WCOW does not require a
// guest-side notification as part of vPCI device assignment.
func (c *Controller) waitGuestDeviceReady(_ context.Context, _ guid.GUID) error {
	return nil
}
