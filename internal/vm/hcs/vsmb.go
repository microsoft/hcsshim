//go:build windows

package hcs

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/vm"
)

func (uvmb *utilityVMBuilder) AddVSMB(ctx context.Context, path string, name string, allowed []string, options *vm.VSMBOptions) error {
	uvmb.doc.VirtualMachine.Devices.VirtualSmb = &hcsschema.VirtualSmb{
		DirectFileMappingInMB: 1024, // Sensible default, but could be a tuning parameter somewhere
		Shares: []hcsschema.VirtualSmbShare{
			{
				Name:         name,
				Path:         path,
				AllowedFiles: allowed,
				Options:      vmVSMBOptionsToHCS(options),
			},
		},
	}
	return nil
}

func (uvmb *utilityVMBuilder) RemoveVSMB(ctx context.Context, name string) error {
	return vm.ErrNotSupported
}

func vmVSMBOptionsToHCS(options *vm.VSMBOptions) *hcsschema.VirtualSmbShareOptions {
	return &hcsschema.VirtualSmbShareOptions{
		ReadOnly:            options.ReadOnly,
		ShareRead:           options.ShareRead,
		CacheIO:             options.CacheIo,
		NoOplocks:           options.NoOplocks,
		NoDirectmap:         options.NoDirectMap,
		TakeBackupPrivilege: options.TakeBackupPrivilege,
		PseudoOplocks:       options.PseudoOplocks,
		PseudoDirnotify:     options.PseudoDirnotify,
	}
}

func (uvm *utilityVM) AddVSMB(ctx context.Context, path string, name string, allowed []string, options *vm.VSMBOptions) error {
	request, err := hcsschema.NewModifySettingRequest(
		resourcepaths.VSMBShareResourcePath,
		hcsschema.ModifyRequestType_ADD,
		hcsschema.VirtualSmbShare{
			Name:         name,
			Options:      vmVSMBOptionsToHCS(options),
			Path:         path,
			AllowedFiles: allowed,
		},
		nil, // guestRequest
	)
	if err != nil {
		return err
	}

	return uvm.cs.Modify(ctx, &request)
}

func (uvm *utilityVM) RemoveVSMB(ctx context.Context, name string) error {
	request, err := hcsschema.NewModifySettingRequest(
		resourcepaths.VSMBShareResourcePath,
		hcsschema.ModifyRequestType_REMOVE,
		hcsschema.VirtualSmbShare{Name: name},
		nil, // guestRequest
	)
	if err != nil {
		return err
	}
	return uvm.cs.Modify(ctx, &request)
}
