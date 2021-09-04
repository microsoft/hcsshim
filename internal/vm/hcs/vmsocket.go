package hcs

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/vm"
	"github.com/pkg/errors"
)

func (uvm *utilityVM) VMSocketListen(ctx context.Context, listenType vm.VMSocketType, connID interface{}) (net.Listener, error) {
	switch listenType {
	case vm.HvSocket:
		serviceGUID, ok := connID.(guid.GUID)
		if !ok {
			return nil, errors.New("parameter passed to hvsocketlisten is not a GUID")
		}
		return uvm.hvSocketListen(ctx, serviceGUID)
	case vm.VSock:
		port, ok := connID.(uint32)
		if !ok {
			return nil, errors.New("parameter passed to vsocklisten is not the right type")
		}
		return uvm.vsockListen(ctx, port)
	default:
		return nil, errors.New("unknown vmsocket type requested")
	}
}

func (uvm *utilityVM) hvSocketListen(ctx context.Context, serviceID guid.GUID) (net.Listener, error) {
	return winio.ListenHvsock(&winio.HvsockAddr{
		VMID:      uvm.vmID,
		ServiceID: serviceID,
	})
}

func (uvm *utilityVM) vsockListen(ctx context.Context, port uint32) (net.Listener, error) {
	return nil, vm.ErrNotSupported
}
