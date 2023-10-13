//go:build windows

package uvm

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// UpdateCPULimits updates the CPU limits of the utility vm
func (uvm *UtilityVM) UpdateCPULimits(ctx context.Context, limits *hcsschema.ProcessorLimits) error {
	req, err := hcsschema.NewModifySettingRequest(
		resourcepaths.CPULimitsResourcePath,
		hcsschema.ModifyRequestType_UPDATE,
		limits,
		nil, // guestRequest
	)
	if err != nil {
		return err
	}
	req.RequestType = nil // request type is unneeded, so remove it

	return uvm.modify(ctx, &req)
}
