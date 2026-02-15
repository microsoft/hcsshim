//go:build windows

package builder

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/vm"

	"github.com/pkg/errors"
)

var (
	errAlreadySet     = errors.New("field has already been set")
	errUnknownGuestOS = errors.New("unknown guest operating system supplied")
)

// UtilityVM is used to build a schema document for creating a Utility VM.
// It provides methods for configuring various aspects of the Utility VM
// such as memory, processors, devices, boot options, and storage QoS settings.
type UtilityVM struct {
	guestOS         vm.GuestOS
	doc             *hcsschema.ComputeSystem
	assignedDevices map[hcsschema.VirtualPciFunction]*vPCIDevice
}

// New returns the concrete builder, and callers are expected to use the
// interface views (for example, NumaOptions, MemoryOptions) as needed.
// This follows the "accept interfaces, return structs" convention.
func New(owner string, guestOS vm.GuestOS) (*UtilityVM, error) {
	doc := &hcsschema.ComputeSystem{
		Owner:         owner,
		SchemaVersion: schemaversion.SchemaV21(),
		// Terminate the UVM when the last handle is closed.
		// When we need to support impactless updates this will need to be configurable.
		ShouldTerminateOnLastHandleClosed: true,
		VirtualMachine: &hcsschema.VirtualMachine{
			StopOnReset: true,
			Chipset:     &hcsschema.Chipset{},
			ComputeTopology: &hcsschema.Topology{
				Memory:    &hcsschema.VirtualMachineMemory{},
				Processor: &hcsschema.VirtualMachineProcessor{},
			},
			Devices: &hcsschema.Devices{
				HvSocket: &hcsschema.HvSocket2{
					HvSocketConfig: &hcsschema.HvSocketSystemConfig{
						// Allow administrators and SYSTEM to bind to vsock sockets
						// so that we can create a GCS log socket.
						DefaultBindSecurityDescriptor: "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
						ServiceTable:                  make(map[string]hcsschema.HvSocketServiceConfig),
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
		return nil, errUnknownGuestOS
	}

	return &UtilityVM{
		guestOS:         guestOS,
		doc:             doc,
		assignedDevices: make(map[hcsschema.VirtualPciFunction]*vPCIDevice),
	}, nil
}

func (uvmb *UtilityVM) GuestOS() vm.GuestOS {
	return uvmb.guestOS
}

func (uvmb *UtilityVM) Get() *hcsschema.ComputeSystem {
	return uvmb.doc
}
