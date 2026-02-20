//go:build windows

package builder

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"

	"github.com/pkg/errors"
)

var (
	errAlreadySet = errors.New("field has already been set")
)

// UtilityVM is used to build a schema document for creating a Utility VM.
// It provides methods for configuring various aspects of the Utility VM
// such as memory, processors, devices, boot options, and storage QoS settings.
type UtilityVM struct {
	doc             *hcsschema.ComputeSystem
	assignedDevices map[hcsschema.VirtualPciFunction]*vPCIDevice
}

// New returns the concrete builder, and callers are expected to use the
// interface views (for example, NumaOptions, MemoryOptions) as needed.
// This follows the "accept interfaces, return structs" convention.
func New(owner string) (*UtilityVM, error) {
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

	return &UtilityVM{
		doc:             doc,
		assignedDevices: make(map[hcsschema.VirtualPciFunction]*vPCIDevice),
	}, nil
}

func (uvmb *UtilityVM) Get() *hcsschema.ComputeSystem {
	return uvmb.doc
}
