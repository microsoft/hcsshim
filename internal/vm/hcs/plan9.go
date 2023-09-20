//go:build windows

package hcs

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

func (uvm *utilityVM) AddPlan9(ctx context.Context, path, name string, port int32, flags int32, allowed []string) error {

	request, err := hcsschema.NewModifySettingRequest(
		resourcepaths.Plan9ShareResourcePath,
		hcsschema.ModifyRequestType_ADD,
		hcsschema.Plan9Share{
			Name:         name,
			AccessName:   name,
			Path:         path,
			Port:         uint32(port),
			Flags:        int64(flags),
			AllowedFiles: allowed,
		},
		nil, // guestRequest
	)
	if err != nil {
		return err
	}
	return uvm.cs.Modify(ctx, request)
}

func (uvm *utilityVM) RemovePlan9(ctx context.Context, name string, port int32) error {
	request, err := hcsschema.NewModifySettingRequest(
		resourcepaths.Plan9ShareResourcePath,
		hcsschema.ModifyRequestType_REMOVE,
		hcsschema.Plan9Share{
			Name:       name,
			AccessName: name,
			Port:       uint32(port),
		},
		nil, // guestRequest
	)
	if err != nil {
		return err
	}
	return uvm.cs.Modify(ctx, request)
}
