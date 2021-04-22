package hcs

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/requesttype"
)

func (uvm *utilityVM) AddPlan9(ctx context.Context, path, name string, port int32, flags int32, allowed []string) error {
	modification := &hcsschema.ModifySettingRequest{
		RequestType: requesttype.Add,
		Settings: hcsschema.Plan9Share{
			Name:         name,
			AccessName:   name,
			Path:         path,
			Port:         port,
			Flags:        flags,
			AllowedFiles: allowed,
		},
		ResourcePath: resourcepaths.Plan9ShareResourcePath,
	}
	return uvm.cs.Modify(ctx, modification)
}

func (uvm *utilityVM) RemovePlan9(ctx context.Context, name string, port int32) error {
	modification := &hcsschema.ModifySettingRequest{
		RequestType: requesttype.Remove,
		Settings: hcsschema.Plan9Share{
			Name:       name,
			AccessName: name,
			Port:       port,
		},
		ResourcePath: resourcepaths.Plan9ShareResourcePath,
	}
	return uvm.cs.Modify(ctx, modification)
}
