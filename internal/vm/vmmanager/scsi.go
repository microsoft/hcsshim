//go:build windows

package vmmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

// SCSIManager manages adding and removing SCSI devices for a Utility VM.
type SCSIManager interface {
	// AddSCSIDisk hot adds a SCSI disk to the Utility VM.
	AddSCSIDisk(ctx context.Context, disk hcsschema.Attachment, controller uint, lun uint) error

	// RemoveSCSIDisk removes a SCSI disk from a Utility VM.
	RemoveSCSIDisk(ctx context.Context, controller uint, lun uint) error
}

var _ SCSIManager = (*UtilityVM)(nil)

func (uvm *UtilityVM) AddSCSIDisk(ctx context.Context, disk hcsschema.Attachment, controller uint, lun uint) error {
	request := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeAdd,
		Settings:     disk,
		ResourcePath: fmt.Sprintf(resourcepaths.SCSIResourceFormat, guestrequest.ScsiControllerGuids[controller], lun),
	}
	if err := uvm.cs.Modify(ctx, request); err != nil {
		return fmt.Errorf("failed to add SCSI disk %s: %w", disk.Path, err)
	}

	return nil
}

func (uvm *UtilityVM) RemoveSCSIDisk(ctx context.Context, controller uint, lun uint) error {
	request := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeRemove,
		ResourcePath: fmt.Sprintf(resourcepaths.SCSIResourceFormat, guestrequest.ScsiControllerGuids[controller], lun),
	}

	if err := uvm.cs.Modify(ctx, request); err != nil {
		return fmt.Errorf("failed to remove SCSI disk %s: %w", request.ResourcePath, err)
	}
	return nil
}
