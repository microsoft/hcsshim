//go:build windows

package hcs

import (
	"context"
	"fmt"
	"strconv"

	"github.com/pkg/errors"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/vm"
)

func (uvmb *utilityVMBuilder) AddSCSIController(id uint32) error {
	if uvmb.doc.VirtualMachine.Devices.SCSI == nil {
		uvmb.doc.VirtualMachine.Devices.SCSI = make(map[string]hcsschema.SCSI, 1)
	}
	uvmb.doc.VirtualMachine.Devices.SCSI[strconv.Itoa(int(id))] = hcsschema.SCSI{
		Attachments: make(map[string]hcsschema.Attachment),
	}
	return nil
}

func (uvmb *utilityVMBuilder) AddSCSIDisk(ctx context.Context, controller uint32, lun uint32, path string, typ vm.SCSIDiskType, readOnly bool) error {
	diskType, err := getSCSIDiskTypeString(typ)
	if err != nil {
		return err
	}

	if uvmb.doc.VirtualMachine.Devices.SCSI == nil {
		return errors.New("no SCSI controller found")
	}

	ctrl, ok := uvmb.doc.VirtualMachine.Devices.SCSI[strconv.Itoa(int(controller))]
	if !ok {
		return fmt.Errorf("no scsi controller with index %d found", controller)
	}

	ctrl.Attachments[strconv.Itoa(int(lun))] = hcsschema.Attachment{
		Path:     path,
		Type_:    &diskType,
		ReadOnly: readOnly,
	}

	return nil
}

func (uvmb *utilityVMBuilder) RemoveSCSIDisk(ctx context.Context, controller uint32, lun uint32, path string) error {
	return vm.ErrNotSupported
}

func (uvm *utilityVM) AddSCSIController(id uint32) error {
	return vm.ErrNotSupported
}

func (uvm *utilityVM) AddSCSIDisk(ctx context.Context, controller uint32, lun uint32, path string, typ vm.SCSIDiskType, readOnly bool) error {
	diskType, err := getSCSIDiskTypeString(typ)
	if err != nil {
		return err
	}

	request, err := hcsschema.NewModifySettingRequest(
		fmt.Sprintf(resourcepaths.SCSIResourceFormat, strconv.Itoa(int(controller)), lun),
		hcsschema.ModifyRequestType_ADD,
		hcsschema.Attachment{
			Path:     path,
			Type_:    &diskType,
			ReadOnly: readOnly,
		},
		nil, // guestRequest
	)
	if err != nil {
		return err
	}

	return uvm.cs.Modify(ctx, request)
}

func (uvm *utilityVM) RemoveSCSIDisk(ctx context.Context, controller uint32, lun uint32, path string) error {
	rt := hcsschema.ModifyRequestType_REMOVE
	request := &hcsschema.ModifySettingRequest{
		RequestType:  &rt,
		ResourcePath: fmt.Sprintf(resourcepaths.SCSIResourceFormat, strconv.Itoa(int(controller)), lun),
	}

	return uvm.cs.Modify(ctx, request)
}

func getSCSIDiskTypeString(typ vm.SCSIDiskType) (hcsschema.AttachmentType, error) {
	switch typ {
	case vm.SCSIDiskTypeVHD1:
		fallthrough
	case vm.SCSIDiskTypeVHDX:
		return hcsschema.AttachmentType_VIRTUAL_DISK, nil
	case vm.SCSIDiskTypePassThrough:
		return hcsschema.AttachmentType_PASS_THRU, nil
	default:
		return "", fmt.Errorf("unsupported SCSI disk type: %d", typ)
	}
}
