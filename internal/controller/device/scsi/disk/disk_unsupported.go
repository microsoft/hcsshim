//go:build windows && !wcow && !lcow

package disk

import (
	"context"
	"fmt"
)

// GuestSCSIEjector is not supported for unsupported guests.
// This is a stub with no method available for use.
type GuestSCSIEjector interface{}

// ejectFromGuest is not supported for unsupported guests.
func (d *Disk) ejectFromGuest(_ context.Context, _ GuestSCSIEjector) error {
	return fmt.Errorf("unsupported guest")
}
