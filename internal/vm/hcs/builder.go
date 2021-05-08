package hcs

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/hcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/vm"
	"github.com/pkg/errors"
)

type utilityVMBuilder struct {
	id      string
	guestOS vm.GuestOS
	doc     *hcsschema.ComputeSystem
}

func NewUVMBuilder(id string, owner string, guestOS vm.GuestOS) (vm.UVMBuilder, error) {
	doc := &hcsschema.ComputeSystem{
		Owner:                             owner,
		SchemaVersion:                     schemaversion.SchemaV21(),
		ShouldTerminateOnLastHandleClosed: true,
		VirtualMachine: &hcsschema.VirtualMachine{
			StopOnReset: true,
			Chipset:     &hcsschema.Chipset{},
			ComputeTopology: &hcsschema.Topology{
				Memory: &hcsschema.Memory2{
					AllowOvercommit: true,
				},
				Processor: &hcsschema.Processor2{},
			},
			Devices: &hcsschema.Devices{
				HvSocket: &hcsschema.HvSocket2{
					HvSocketConfig: &hcsschema.HvSocketSystemConfig{
						// Allow administrators and SYSTEM to bind to vsock sockets
						// so that we can create a GCS log socket.
						DefaultBindSecurityDescriptor: "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
					},
				},
			},
		},
	}

	switch guestOS {
	case vm.Windows:
		doc.VirtualMachine.Devices.VirtualSmb = &hcsschema.VirtualSmb{}
	case vm.Linux:
		doc.VirtualMachine.Devices.Plan9 = &hcsschema.Plan9{}
	default:
		return nil, vm.ErrUnknownGuestOS
	}

	return &utilityVMBuilder{
		id:      id,
		guestOS: guestOS,
		doc:     doc,
	}, nil
}

func (uvmb *utilityVMBuilder) Create(ctx context.Context) (_ vm.UVM, err error) {
	cs, err := hcs.CreateComputeSystem(ctx, uvmb.id, uvmb.doc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create hcs compute system")
	}

	defer func() {
		if err != nil {
			_ = cs.Terminate(ctx)
			_ = cs.Wait()
		}
	}()

	backingType := vm.MemoryBackingTypeVirtual
	if !uvmb.doc.VirtualMachine.ComputeTopology.Memory.AllowOvercommit {
		backingType = vm.MemoryBackingTypePhysical
	}

	uvm := &utilityVM{
		id:          uvmb.id,
		guestOS:     uvmb.guestOS,
		cs:          cs,
		backingType: backingType,
		state:       vm.StateCreated,
	}

	properties, err := cs.Properties(ctx)
	if err != nil {
		return nil, err
	}
	uvm.vmID = properties.RuntimeID
	return uvm, nil
}
