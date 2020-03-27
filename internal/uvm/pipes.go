package uvm

import (
	"context"
	"fmt"
	"strings"

	"github.com/Microsoft/hcsshim/internal/requesttype"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	specs "github.com/opencontainers/runtime-spec/specs-go"
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
		return fmt.Errorf("failed to remove pipe mount: %s", err)
	}
	return nil
}

// AddPipe shares a named pipe into the UVM.
func (uvm *UtilityVM) AddPipe(ctx context.Context, hostPath string) (*PipeMount, error) {
	modification := &hcsschema.ModifySettingRequest{
		RequestType:  requesttype.Add,
		ResourcePath: fmt.Sprintf(mappedPipeResourceFormat, hostPath),
	}
	if err := uvm.modify(ctx, modification); err != nil {
		return nil, err
	}
	return &PipeMount{uvm, hostPath}, nil
}

// RemovePipe removes a shared named pipe from the UVM.
func (uvm *UtilityVM) RemovePipe(ctx context.Context, hostPath string) error {
	modification := &hcsschema.ModifySettingRequest{
		RequestType:  requesttype.Remove,
		ResourcePath: fmt.Sprintf(mappedPipeResourceFormat, hostPath),
	}
	if err := uvm.modify(ctx, modification); err != nil {
		return err
	}
	return nil
}

// IsPipe returns true if the given path references a named pipe.
func IsPipe(hostPath string) bool {
	return strings.HasPrefix(hostPath, pipePrefix)
}

// GetContainerPipeMapping returns the source and destination to use for a given
// pipe mount in a container.
func GetContainerPipeMapping(uvm *UtilityVM, mount specs.Mount) (src string, dst string) {
	if uvm == nil {
		src = mount.Source
	} else {
		src = vsmbSharePrefix + `IPC$\` + strings.TrimPrefix(mount.Source, pipePrefix)
	}
	dst = strings.TrimPrefix(mount.Destination, pipePrefix)
	return src, dst
}
