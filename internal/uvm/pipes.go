//go:build windows

package uvm

import (
	"context"
	"fmt"
	"strings"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/opencontainers/runtime-spec/specs-go"
)

const pipePrefix = `\\.\pipe\`

// PipeMount contains the host path for pipe mount
type PipeMount struct {
	// UVM the resource belongs to
	vm       *UtilityVM
	HostPath string
}

// Release frees the resources of the corresponding pipe Mount
func (pipe *PipeMount) Release(ctx context.Context) error {
	if err := pipe.vm.RemovePipe(ctx, pipe.HostPath); err != nil {
		return fmt.Errorf("failed to remove pipe mount: %w", err)
	}
	return nil
}

// AddPipe shares a named pipe into the UVM.
func (uvm *UtilityVM) AddPipe(ctx context.Context, hostPath string) (*PipeMount, error) {
	modification := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeAdd,
		ResourcePath: fmt.Sprintf(resourcepaths.MappedPipeResourceFormat, hostPath),
	}
	if err := uvm.modify(ctx, modification); err != nil {
		return nil, err
	}
	return &PipeMount{uvm, hostPath}, nil
}

// RemovePipe removes a shared named pipe from the UVM.
func (uvm *UtilityVM) RemovePipe(ctx context.Context, hostPath string) error {
	modification := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeRemove,
		ResourcePath: fmt.Sprintf(resourcepaths.MappedPipeResourceFormat, hostPath),
	}
	if err := uvm.modify(ctx, modification); err != nil {
		return err
	}
	return nil
}

// UVMNamedPipe returns a named pipe in UVM, which will be shared across containers.
func (uvm *UtilityVM) UVMNamedPipe(hostPath string) string {
	podID := strings.TrimSuffix(uvm.id, "@vm")
	if uvm.operatingSystem == "windows" {
		uvmPipeName := strings.TrimPrefix(hostPath, pipePrefix)
		return fmt.Sprintf(`%s%s\%s`, pipePrefix, podID, uvmPipeName)
	}
	// TODO (anmaxvl): LCOW doesn't support UVM named pipes at the moment
	return hostPath
}

// IsPipe returns true if the given path references a named pipe. The pipe can be:
// - host named pipe shared via VSMB
func IsPipe(hostPath string) bool {
	return strings.HasPrefix(hostPath, pipePrefix)
}

// GetContainerPipeMapping returns the source and destination to use for a given
// pipe mount in a container.
// The pipe mount can be either a host pipe shared via VSMB or a UVM pipe.
func GetContainerPipeMapping(uvm *UtilityVM, mount specs.Mount) (src string, dst string) {
	if uvm == nil {
		src = mount.Source
	} else {
		if uvmPipe, ok := strings.CutPrefix(mount.Source, guestpath.UVMMountPrefix); ok {
			src = uvm.UVMNamedPipe(uvmPipe)
		} else {
			src = vsmbSharePrefix + `IPC$\` + strings.TrimPrefix(mount.Source, pipePrefix)
		}
	}
	dst = strings.TrimPrefix(mount.Destination, pipePrefix)
	return src, dst
}
