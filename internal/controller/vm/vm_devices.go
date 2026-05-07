//go:build windows && (lcow || wcow)

package vm

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi"
	"github.com/Microsoft/hcsshim/internal/controller/device/vpci"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"
)

// SCSIController returns the singleton SCSI device controller for this VM.
func (c *Controller) SCSIController(ctx context.Context) (*scsi.Controller, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.scsiController == nil {
		uvm := c.uvm.(*vmmanager.UtilityVM)
		guest := c.guest.(*guestmanager.Guest)
		ctrl, err := newSCSIController(ctx, c.hcsDocument, uvm, guest)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize SCSI controller: %w", err)
		}
		c.scsiController = ctrl
	}
	return c.scsiController, nil
}

// VPCIController returns the singleton vPCI device controller for this VM.
func (c *Controller) VPCIController() *vpci.Controller {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.vpciController == nil {
		uvm := c.uvm.(*vmmanager.UtilityVM)
		guest := c.guest.(*guestmanager.Guest)
		c.vpciController = vpci.New(uvm, guest)
	}

	return c.vpciController
}

// newSCSIController creates a [scsi.Controller] from the HCS document,
// pre-reserving every rootfs slot that already has an attachment in the document.
func newSCSIController(
	ctx context.Context,
	doc *hcsschema.ComputeSystem,
	vm scsi.VMSCSIOps,
	guest scsi.GuestSCSIOps,
) (*scsi.Controller, error) {
	// If there are no SCSI device controllers in the document, error out.
	if doc.VirtualMachine == nil ||
		doc.VirtualMachine.Devices == nil ||
		len(doc.VirtualMachine.Devices.Scsi) == 0 {
		return nil, fmt.Errorf("expected the VM to have at least one SCSI controller")
	}

	// Create a VM SCSI controller.
	scsiMap := doc.VirtualMachine.Devices.Scsi
	ctrl := scsi.New(len(scsiMap), vm, guest)

	// Iterate over the well-known controller GUIDs so the slice index gives us
	// the correct controller number directly.
	for ctrlIdx, guid := range guestrequest.ScsiControllerGuids {
		c, ok := scsiMap[guid]
		if !ok {
			continue
		}

		// Found the controller GUID in the document.
		for lunStr := range c.Attachments {
			lun, err := strconv.ParseUint(lunStr, 10, 32)
			if err != nil {
				continue
			}

			if err := ctrl.ReserveForRootfs(ctx, uint(ctrlIdx), uint(lun)); err != nil {
				return nil, fmt.Errorf("reserve SCSI slot (controller=%d, lun=%d): %w", ctrlIdx, lun, err)
			}
		}
	}

	return ctrl, nil
}
