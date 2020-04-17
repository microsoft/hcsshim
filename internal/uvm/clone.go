package uvm

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/pkg/errors"
)

const (
	hcsSaveOptions = "{\"SaveType\": \"AsTemplate\"}"
)

type TemplateVSMBConfig struct {
	ShareName string
	HostPath  string
}

type TemplateSCSIMountConfig struct {
	Controller int
	LUN        int32
	HostPath   string
	Type       string
	// if this is a scratch layer then replace it during clone
	IsScratch bool
}

type UVMTemplateConfig struct {
	// ID of the template vm
	UVMID string
	// Path of the UVM VHDpath.
	UVMVhdPath string
	// All of the VSMB mounts attached to this template uvm
	VSMBShares []TemplateVSMBConfig
	// All of the SCSI mounts attached to this template uvm
	SCSIMounts []TemplateSCSIMountConfig
}

// ID returns the ID of the VM's compute system.
func (uvm *UtilityVM) SaveAsTemplate(ctx context.Context) error {
	err := uvm.hcsSystem.Pause(ctx)
	if err != nil {
		return errors.Wrapf(err, "Error pausing the VM")
	}

	err = uvm.hcsSystem.Save(ctx, hcsSaveOptions)
	if err != nil {
		return errors.Wrapf(err, "Error saving the VM")
	}
	return nil
}

// CloneContainer attaches back to a container that is already running inside the UVM
// because of the clone
func (uvm *UtilityVM) CloneContainer(ctx context.Context, id string, settings interface{}) (cow.Container, error) {
	if uvm.gc == nil {
		return nil, fmt.Errorf("Clone container cannot work without external GCS connection")
	}
	c, err := uvm.gc.CloneContainer(ctx, id, settings)
	if err != nil {
		return nil, fmt.Errorf("failed to clone container %s: %s", id, err)
	}
	return c, nil
}
