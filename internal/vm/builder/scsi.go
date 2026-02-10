//go:build windows

package builder

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/pkg/errors"
)

func (uvmb *UtilityVM) AddSCSIController(id string) {
	if uvmb.doc.VirtualMachine.Devices.Scsi == nil {
		uvmb.doc.VirtualMachine.Devices.Scsi = make(map[string]hcsschema.Scsi)
	}
	uvmb.doc.VirtualMachine.Devices.Scsi[id] = hcsschema.Scsi{
		Attachments: make(map[string]hcsschema.Attachment),
	}
}

func (uvmb *UtilityVM) AddSCSIDisk(controller string, lun string, disk hcsschema.Attachment) error {
	if uvmb.doc.VirtualMachine.Devices.Scsi == nil {
		return errors.New("SCSI controller has not been added")
	}

	ctrl, ok := uvmb.doc.VirtualMachine.Devices.Scsi[controller]
	if !ok {
		return errors.Errorf("no scsi controller with id %s found", controller)
	}

	ctrl.Attachments[lun] = disk
	uvmb.doc.VirtualMachine.Devices.Scsi[controller] = ctrl

	return nil
}
