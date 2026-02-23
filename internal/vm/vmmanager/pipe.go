//go:build windows

package vmmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

// PipeManager manages adding and removing named pipes for a Utility VM.
type PipeManager interface {
	// AddPipe adds a named pipe to the Utility VM.
	AddPipe(ctx context.Context, hostPath string) error

	// RemovePipe removes a named pipe from the Utility VM.
	RemovePipe(ctx context.Context, hostPath string) error
}

var _ PipeManager = (*UtilityVM)(nil)

func (uvm *UtilityVM) AddPipe(ctx context.Context, hostPath string) error {
	modification := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeAdd,
		ResourcePath: fmt.Sprintf(resourcepaths.MappedPipeResourceFormat, hostPath),
	}
	if err := uvm.cs.Modify(ctx, modification); err != nil {
		return fmt.Errorf("failed to add pipe %s to uvm: %w", hostPath, err)
	}

	return nil
}

func (uvm *UtilityVM) RemovePipe(ctx context.Context, hostPath string) error {
	modification := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeRemove,
		ResourcePath: fmt.Sprintf(resourcepaths.MappedPipeResourceFormat, hostPath),
	}
	if err := uvm.cs.Modify(ctx, modification); err != nil {
		return fmt.Errorf("failed to remove pipe %s from uvm: %w", hostPath, err)
	}

	return nil
}
