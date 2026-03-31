//go:build windows && lcow

package vpci

import (
	"context"

	"github.com/Microsoft/go-winio/pkg/guid"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// waitGuestDeviceReady notifies the guest about the new device and blocks until
// the required sysfs/device paths are available before workloads use them.
func (c *Controller) waitGuestDeviceReady(ctx context.Context, vmBusGUID guid.GUID) error {
	return c.linuxGuestVPCI.AddVPCIDevice(ctx, guestresource.LCOWMappedVPCIDevice{
		VMBusGUID: vmBusGUID.String(),
	})
}
