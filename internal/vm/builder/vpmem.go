//go:build windows

package builder

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/pkg/errors"
)

func (uvmb *UtilityVM) AddVPMemController(maximumDevices uint32, maximumSizeBytes uint64) {
	uvmb.doc.VirtualMachine.Devices.VirtualPMem = &hcsschema.VirtualPMemController{
		MaximumCount:     maximumDevices,
		MaximumSizeBytes: maximumSizeBytes,
		Devices:          make(map[string]hcsschema.VirtualPMemDevice),
	}
}

func (uvmb *UtilityVM) AddVPMemDevice(id string, device hcsschema.VirtualPMemDevice) error {
	if uvmb.doc.VirtualMachine.Devices.VirtualPMem == nil {
		return errors.New("VPMem controller has not been added")
	}
	uvmb.doc.VirtualMachine.Devices.VirtualPMem.Devices[id] = device
	return nil
}
