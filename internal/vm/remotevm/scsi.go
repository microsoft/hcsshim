package remotevm

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/vm"
	"github.com/Microsoft/hcsshim/internal/vmservice"
	"github.com/pkg/errors"
)

func getSCSIDiskType(typ vm.SCSIDiskType) (vmservice.DiskType, error) {
	var diskType vmservice.DiskType
	switch typ {
	case vm.SCSIDiskTypeVHD1:
		diskType = vmservice.DiskType_SCSI_DISK_TYPE_VHD1
	case vm.SCSIDiskTypeVHDX:
		diskType = vmservice.DiskType_SCSI_DISK_TYPE_VHDX
	case vm.SCSIDiskTypePassThrough:
		diskType = vmservice.DiskType_SCSI_DISK_TYPE_PHYSICAL
	default:
		return -1, fmt.Errorf("unsupported SCSI disk type: %d", typ)
	}
	return diskType, nil
}

func (uvmb *utilityVMBuilder) AddSCSIController(id uint32) error {
	return nil
}

func (uvmb *utilityVMBuilder) AddSCSIDisk(ctx context.Context, controller, lun uint32, path string, typ vm.SCSIDiskType, readOnly bool) error {
	diskType, err := getSCSIDiskType(typ)
	if err != nil {
		return err
	}

	disk := &vmservice.SCSIDisk{
		Controller: controller,
		Lun:        lun,
		HostPath:   path,
		Type:       diskType,
		ReadOnly:   readOnly,
	}

	uvmb.config.DevicesConfig.ScsiDisks = append(uvmb.config.DevicesConfig.ScsiDisks, disk)
	return nil
}

func (uvmb *utilityVMBuilder) RemoveSCSIDisk(ctx context.Context, controller, lun uint32, path string) error {
	return vm.ErrNotSupported
}

func (uvm *utilityVM) AddSCSIController(id uint32) error {
	return vm.ErrNotSupported
}

func (uvm *utilityVM) AddSCSIDisk(ctx context.Context, controller, lun uint32, path string, typ vm.SCSIDiskType, readOnly bool) error {
	diskType, err := getSCSIDiskType(typ)
	if err != nil {
		return err
	}

	disk := &vmservice.SCSIDisk{
		Controller: controller,
		Lun:        lun,
		HostPath:   path,
		Type:       diskType,
		ReadOnly:   readOnly,
	}

	if _, err := uvm.client.ModifyResource(ctx,
		&vmservice.ModifyResourceRequest{
			Type: vmservice.ModifyType_ADD,
			Resource: &vmservice.ModifyResourceRequest_ScsiDisk{
				ScsiDisk: disk,
			},
		},
	); err != nil {
		return errors.Wrap(err, "failed to add SCSI disk")
	}

	return nil
}

func (uvm *utilityVM) RemoveSCSIDisk(ctx context.Context, controller, lun uint32, path string) error {
	disk := &vmservice.SCSIDisk{
		Controller: controller,
		Lun:        lun,
		HostPath:   path,
	}

	if _, err := uvm.client.ModifyResource(ctx,
		&vmservice.ModifyResourceRequest{
			Type: vmservice.ModifyType_REMOVE,
			Resource: &vmservice.ModifyResourceRequest_ScsiDisk{
				ScsiDisk: disk,
			},
		},
	); err != nil {
		return errors.Wrapf(err, "failed to remove SCSI disk %q", path)
	}

	return nil
}
