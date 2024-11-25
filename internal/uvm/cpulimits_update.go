//go:build windows

package uvm

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/containerd/errdefs"
)

// UpdateCPULimits updates the CPU limits of the utility vm
func (uvm *UtilityVM) UpdateCPULimits(ctx context.Context, limits *hcsschema.ProcessorLimits) error {
	// Support for updating CPU limits was not added until 20H2 build
	if osversion.Get().Build < osversion.V20H2 {
		return errdefs.ErrNotImplemented
	}
	req := &hcsschema.ModifySettingRequest{
		ResourcePath: resourcepaths.CPULimitsResourcePath,
		Settings:     limits,
	}

	return uvm.modify(ctx, req)
}
