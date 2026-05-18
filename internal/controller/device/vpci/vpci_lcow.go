//go:build windows && lcow

package vpci

import (
	"context"

	"github.com/Microsoft/go-winio/pkg/guid"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// guestVPCI exposes vPCI device operations in the LCOW guest.
type guestVPCI interface {
	// AddVPCIDevice adds a vPCI device to the guest.
	AddVPCIDevice(ctx context.Context, settings guestresource.LCOWMappedVPCIDevice) error
}

// waitGuestDeviceReady notifies the guest about the new device and blocks until
// the required sysfs/device paths are available before workloads use them.
func (c *Controller) waitGuestDeviceReady(ctx context.Context, vmBusGUID guid.GUID) error {
	return c.guestVPCI.AddVPCIDevice(ctx, guestresource.LCOWMappedVPCIDevice{
		VMBusGUID: vmBusGUID.String(),
	})
}
