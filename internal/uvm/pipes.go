//go:build windows

package uvm

import (
	"context"
	"fmt"
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
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

type NamedPipe struct {
	HostPath      string
	ContainerPath string
	UVMPipe       bool
}

// IsPipe returns true if the given path references a named pipe. The pipe can be:
// - host named pipe shared via VSMB
// - UVM named pipe
func IsPipe(hostPath string) bool {
	if pipePath, ok := strings.CutPrefix(hostPath, guestpath.UVMMountPrefix); ok {
		return strings.HasPrefix(pipePath, pipePrefix)
	}
	return strings.HasPrefix(hostPath, pipePrefix)
}

// ParseNamedPipe tries parses the mount as a named pipe and returns `NamedPipe` mapping
// for a container.
// The pipe mount can be either a host pipe shared via VSMB or a UVM pipe.
func ParseNamedPipe(uvm *UtilityVM, mount specs.Mount) (NamedPipe, bool) {
	// we only support windows mapped pipes at the moment
	if uvm != nil && uvm.operatingSystem != "windows" {
		return NamedPipe{}, false
	}

	// destination is not a named pipe, return false
	if !strings.HasPrefix(mount.Destination, pipePrefix) {
		return NamedPipe{}, false
	}

	// host pipe mapped into container either for process isolated containers or
	// for Hyper-V isolated WCOW over VSMB
	isHostPipe := false
	// UVM pipe mapped into container
	isUVMPipe := false
	sourcePath := mount.Source
	containerPath := strings.TrimPrefix(mount.Destination, pipePrefix)
	if strings.HasPrefix(mount.Source, pipePrefix) {
		isHostPipe = true
		if uvm != nil {
			sourcePath = vsmbSharePrefix + `IPC$\` + strings.TrimPrefix(mount.Source, pipePrefix)
		}
	} else if strings.HasPrefix(mount.Source, guestpath.UVMMountPrefix) {
		sourcePath = strings.TrimPrefix(mount.Source, guestpath.UVMMountPrefix)
		// check if it's a UVM pipe
		if strings.HasPrefix(sourcePath, pipePrefix) && uvm != nil {
			isUVMPipe = true
			podID := strings.TrimSuffix(uvm.id, "@vm")
			sourcePath = fmt.Sprintf(`%s%s\%s`, pipePrefix, podID, strings.TrimPrefix(sourcePath, pipePrefix))
		}
	}
	if !isHostPipe && !isUVMPipe {
		return NamedPipe{}, false
	}
	return NamedPipe{
		HostPath:      sourcePath,
		ContainerPath: containerPath,
		UVMPipe:       isUVMPipe,
	}, true
}
