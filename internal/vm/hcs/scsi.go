//go:build windows

package hcs

import (
	"context"
	"fmt"
	"strconv"

	"github.com/pkg/errors"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/vm"
)

func (uvmb *utilityVMBuilder) AddSCSIController(id uint32) error {
	if uvmb.doc.VirtualMachine.Devices.Scsi == nil {
		uvmb.doc.VirtualMachine.Devices.Scsi = make(map[string]hcsschema.Scsi, 1)
	}
	uvmb.doc.VirtualMachine.Devices.Scsi[strconv.Itoa(int(id))] = hcsschema.Scsi{
		Attachments: make(map[string]hcsschema.Attachment),
	}
	return nil
}

func (uvmb *utilityVMBuilder) AddSCSIDisk(ctx context.Context, controller uint32, lun uint32, path string, typ vm.SCSIDiskType, readOnly bool) error {
	if uvmb.doc.VirtualMachine.Devices.Scsi == nil {
		return errors.New("no SCSI controller found")
	}

	ctrl, ok := uvmb.doc.VirtualMachine.Devices.Scsi[strconv.Itoa(int(controller))]
	if !ok {
		return fmt.Errorf("no scsi controller with index %d found", controller)
	}

	ctrl.Attachments[strconv.Itoa(int(lun))] = hcsschema.Attachment{
		Path:     path,
		Type_:    string(typ),
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
	diskTypeString, err := getSCSIDiskTypeString(typ)
	if err != nil {
		return err
	}
	request := &hcsschema.ModifySettingRequest{
		RequestType: guestrequest.RequestTypeAdd,
		Settings: hcsschema.Attachment{
			Path:     path,
			Type_:    diskTypeString,
			ReadOnly: readOnly,
		},
		ResourcePath: fmt.Sprintf(resourcepaths.SCSIResourceFormat, strconv.Itoa(int(controller)), lun),
	}
	return uvm.cs.Modify(ctx, request)
}

func (uvm *utilityVM) RemoveSCSIDisk(ctx context.Context, controller uint32, lun uint32, path string) error {
	request := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeRemove,
		ResourcePath: fmt.Sprintf(resourcepaths.SCSIResourceFormat, strconv.Itoa(int(controller)), lun),
	}

	return uvm.cs.Modify(ctx, request)
}

func getSCSIDiskTypeString(typ vm.SCSIDiskType) (string, error) {
	switch typ {
	case vm.SCSIDiskTypeVHD1:
		fallthrough
	case vm.SCSIDiskTypeVHDX:
		return "VirtualDisk", nil
	case vm.SCSIDiskTypePassThrough:
		return "PassThru", nil
	default:
		return "", fmt.Errorf("unsupported SCSI disk type: %d", typ)
	}
}
