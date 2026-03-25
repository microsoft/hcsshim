//go:build windows && !wcow

package vpci

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// waitGuestDeviceReady notifies the guest about the new device and blocks until
// the required sysfs/device paths are available before workloads use them.
func (m *Manager) waitGuestDeviceReady(ctx context.Context, vmBusGUID string) error {
	return m.linuxGuestVPCI.AddVPCIDevice(ctx, guestresource.LCOWMappedVPCIDevice{
		VMBusGUID: vmBusGUID,
	})
}
