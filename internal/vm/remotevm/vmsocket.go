//go:build windows

package remotevm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/vm"
	"github.com/Microsoft/hcsshim/internal/vmservice"
)

func (uvm *utilityVM) VMSocketListen(ctx context.Context, listenType vm.VMSocketType, connID interface{}) (_ net.Listener, err error) {
	// Make a temp file and delete to "reserve" a unique name for the unix socket
	f, err := os.CreateTemp("", "")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file for unix socket: %w", err)
	}

	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Remove(f.Name()); err != nil {
		return nil, fmt.Errorf("failed to delete temp file to free up name: %w", err)
	}

	l, err := net.Listen("unix", f.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to listen on unix socket %q: %w", f.Name(), err)
	}

	defer func() {
		if err != nil {
			_ = l.Close()
		}
	}()

	switch listenType {
	case vm.HvSocket:
		serviceGUID, ok := connID.(guid.GUID)
		if !ok {
			return nil, errors.New("parameter passed to hvsocketlisten is not a GUID")
		}
		if err := uvm.hvSocketListen(ctx, serviceGUID.String(), f.Name()); err != nil {
			return nil, fmt.Errorf("failed to setup relay to hvsocket listener: %w", err)
		}
	case vm.VSock:
		port, ok := connID.(uint32)
		if !ok {
			return nil, errors.New("parameter passed to vsocklisten is not the right type")
		}
		if err := uvm.vsockListen(ctx, port, f.Name()); err != nil {
			return nil, fmt.Errorf("failed to setup relay to vsock listener: %w", err)
		}
	default:
		return nil, errors.New("unknown vmsocket type requested")
	}

	return l, nil
}

func (uvm *utilityVM) hvSocketListen(ctx context.Context, serviceID string, listenerPath string) error {
	if _, err := uvm.client.VMSocket(ctx, &vmservice.VMSocketRequest{
		Type: vmservice.ModifyType_ADD,
		Config: &vmservice.VMSocketRequest_HvsocketList{
			HvsocketList: &vmservice.HVSocketListen{
				ServiceID:    serviceID,
				ListenerPath: listenerPath,
			},
		},
	}); err != nil {
		return err
	}
	return nil
}

func (uvm *utilityVM) vsockListen(ctx context.Context, port uint32, listenerPath string) error {
	if _, err := uvm.client.VMSocket(ctx, &vmservice.VMSocketRequest{
		Type: vmservice.ModifyType_ADD,
		Config: &vmservice.VMSocketRequest_VsockListen{
			VsockListen: &vmservice.VSockListen{
				Port:         port,
				ListenerPath: listenerPath,
			},
		},
	}); err != nil {
		return err
	}
	return nil
}
