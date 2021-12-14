package hcs

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
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
		CacheIo:             options.CacheIo,
		NoOplocks:           options.NoOplocks,
		NoDirectmap:         options.NoDirectMap,
		TakeBackupPrivilege: options.TakeBackupPrivilege,
		PseudoOplocks:       options.PseudoOplocks,
		PseudoDirnotify:     options.PseudoDirnotify,
	}
}

func (uvm *utilityVM) AddVSMB(ctx context.Context, path string, name string, allowed []string, options *vm.VSMBOptions) error {
	modification := &hcsschema.ModifySettingRequest{
		RequestType: guestrequest.RequestTypeAdd,
		Settings: hcsschema.VirtualSmbShare{
			Name:         name,
			Options:      vmVSMBOptionsToHCS(options),
			Path:         path,
			AllowedFiles: allowed,
		},
		ResourcePath: resourcepaths.VSMBShareResourcePath,
	}
	return uvm.cs.Modify(ctx, modification)
}

func (uvm *utilityVM) RemoveVSMB(ctx context.Context, name string) error {
	modification := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeRemove,
		Settings:     hcsschema.VirtualSmbShare{Name: name},
		ResourcePath: resourcepaths.VSMBShareResourcePath,
	}
	return uvm.cs.Modify(ctx, modification)
}
