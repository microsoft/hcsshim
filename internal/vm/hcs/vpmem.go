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

func (uvmb *utilityVMBuilder) AddVPMemController(maximumDevices uint32, maximumSizeBytes uint64) error {
	uvmb.doc.VirtualMachine.Devices.VirtualPMem = &hcsschema.VirtualPMemController{
		MaximumCount:     uint8(maximumDevices),
		MaximumSizeBytes: maximumSizeBytes,
	}
	uvmb.doc.VirtualMachine.Devices.VirtualPMem.Devices = make(map[string]hcsschema.VirtualPMemDevice)
	return nil
}

func (uvmb *utilityVMBuilder) AddVPMemDevice(ctx context.Context, id uint32, path string, readOnly bool, imageFormat vm.VPMemImageFormat) error {
	if uvmb.doc.VirtualMachine.Devices.VirtualPMem == nil {
		return errors.New("VPMem controller has not been added")
	}
	imageFormatString, err := getVPMemImageFormatString(imageFormat)
	if err != nil {
		return err
	}
	uvmb.doc.VirtualMachine.Devices.VirtualPMem.Devices[strconv.Itoa(int(id))] = hcsschema.VirtualPMemDevice{
		HostPath:    path,
		ReadOnly:    readOnly,
		ImageFormat: &imageFormatString,
	}
	return nil
}

func (uvmb *utilityVMBuilder) RemoveVPMemDevice(ctx context.Context, id uint32, path string) error {
	return vm.ErrNotSupported
}

func (uvm *utilityVM) AddVPMemController(maximumDevices uint32, maximumSizeBytes uint64) error {
	return vm.ErrNotSupported
}

func (uvm *utilityVM) AddVPMemDevice(ctx context.Context, id uint32, path string, readOnly bool, imageFormat vm.VPMemImageFormat) error {
	imageFormatString, err := getVPMemImageFormatString(imageFormat)
	if err != nil {
		return err
	}
	request, err := hcsschema.NewModifySettingRequest(
		fmt.Sprintf(resourcepaths.VPMemControllerResourceFormat, id),
		hcsschema.ModifyRequestType_ADD,
		hcsschema.VirtualPMemDevice{
			HostPath:    path,
			ReadOnly:    readOnly,
			ImageFormat: &imageFormatString,
		},
		nil, // guestRequest
	)
	if err != nil {
		return err
	}
	return uvm.cs.Modify(ctx, &request)
}

func (uvm *utilityVM) RemoveVPMemDevice(ctx context.Context, id uint32, path string) error {
	rt := hcsschema.ModifyRequestType_REMOVE
	request := &hcsschema.ModifySettingRequest{
		RequestType:  &rt,
		ResourcePath: fmt.Sprintf(resourcepaths.VPMemControllerResourceFormat, id),
	}
	return uvm.cs.Modify(ctx, request)
}

func getVPMemImageFormatString(imageFormat vm.VPMemImageFormat) (hcsschema.VirtualPMemImageFormat, error) {
	switch imageFormat {
	case vm.VPMemImageFormatVHD1:
		return hcsschema.VirtualPMemImageFormat_VHD1, nil
	case vm.VPMemImageFormatVHDX:
		return hcsschema.VirtualPMemImageFormat_VHDX, nil
	default:
		return "", fmt.Errorf("unsupported VPMem image format: %d", imageFormat)
	}
}
