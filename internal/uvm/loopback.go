package uvm

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const lcowLoopbackLayerFmt = "/run/layers/lb%d"

// LoopbackDevice represents a loopback device in the guest.
type LoopbackDevice struct {
	// Utility VM the loopback device
	vm *UtilityVM
	// deviceNumber is the device number for the loopback device
	deviceNumber uint32
	// refCount for this loopback device if the same
	refCount uint32
	// backingFile is the backing file for the loopback device. This is the path to
	// a file in the guest and not the host.
	backingFile string
	// path in the UVM the device is mounted at.
	uvmPath string
}

// Release removes the loopback device from the UVM it belongs to.
func (lb *LoopbackDevice) Release(ctx context.Context) error {
	if err := lb.vm.RemoveLoopback(ctx, lb.backingFile); err != nil {
		return errors.Wrapf(err, "failed to remove loopback device with backing file %s", lb.backingFile)
	}
	return nil
}

// findNextLoopback finds the next available loopback device.
//
// The lock MUST be held when calling this function.
func (uvm *UtilityVM) findNextLoopbackDevice(ctx context.Context) uint32 {
	for k, v := range uvm.loopbackDevices {
		if v == nil {
			log.G(ctx).WithFields(logrus.Fields{
				"deviceNumber": k,
			}).Debug("found previous loopback device free")
			return k
		}
	}
	// Didn't find anything free, check what the current mount count is and use this.
	currentMountCount := uvm.loopbackCounter
	uvm.loopbackCounter++
	return currentMountCount
}

// Lock must be held when calling this function
func (uvm *UtilityVM) findLoopbackDevice(ctx context.Context, backingFile string) (*LoopbackDevice, error) {
	for _, v := range uvm.loopbackDevices {
		if v != nil && v.backingFile == backingFile {
			log.G(ctx).WithFields(logrus.Fields{
				"deviceNumber": v.deviceNumber,
				"backingFile":  v.backingFile,
				"UVMPath":      v.uvmPath,
			}).Debug("found loopback device")
			return v, nil
		}
	}
	return nil, ErrNotAttached
}

// AddLoopback adds a loopback device backed with `backingFile` inside the utility VM at
// the next available location and returns the location in the utility VM where it was mounted.
// It's expected that `backingFile` is a path to a file in the utility VM that already exists.
func (uvm *UtilityVM) AddLoopback(ctx context.Context, backingFile string) (_ string, err error) {
	if uvm.operatingSystem != "linux" {
		return "", errNotSupported
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	var deviceNumber uint32
	lb, err := uvm.findLoopbackDevice(ctx, backingFile)
	if err != nil {
		// It doesn't exist so we need to add it.
		deviceNumber = uvm.findNextLoopbackDevice(ctx)
		uvmPath := fmt.Sprintf(lcowLoopbackLayerFmt, deviceNumber)
		modification := &hcsschema.ModifySettingRequest{
			GuestRequest: guestrequest.GuestRequest{
				ResourceType: guestrequest.ResourceTypeLoopbackDevice,
				RequestType:  requesttype.Add,
				Settings: guestrequest.LCOWLoopbackDevice{
					DeviceNumber: deviceNumber,
					MountPath:    uvmPath,
					BackingFile:  backingFile,
				},
			},
		}

		if err := uvm.modify(ctx, modification); err != nil {
			return "", errors.Wrap(err, "failed to modify utility VM to add loopback device")
		}

		uvm.loopbackDevices[deviceNumber] = &LoopbackDevice{
			vm:           uvm,
			deviceNumber: deviceNumber,
			backingFile:  backingFile,
			refCount:     1,
			uvmPath:      uvmPath,
		}
		return uvmPath, nil
	}
	lb.refCount++
	return lb.uvmPath, nil
}

// RemoveLoopback removes a loopback device from a Utility VM. If the `backingFile` isn't mounted,
// returns errNotAttached.
func (uvm *UtilityVM) RemoveLoopback(ctx context.Context, backingFile string) (err error) {
	if uvm.operatingSystem != "linux" {
		return errNotSupported
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	lb, err := uvm.findLoopbackDevice(ctx, backingFile)
	if err != nil {
		return err
	}

	if lb.refCount == 1 {
		modification := &hcsschema.ModifySettingRequest{
			GuestRequest: guestrequest.GuestRequest{
				ResourceType: guestrequest.ResourceTypeLoopbackDevice,
				RequestType:  requesttype.Remove,
				Settings: guestrequest.LCOWLoopbackDevice{
					DeviceNumber: lb.deviceNumber,
					MountPath:    lb.uvmPath,
				},
			},
		}

		if err := uvm.modify(ctx, modification); err != nil {
			return errors.Wrapf(err, "failed to remove loopback device with backing file %s from utility VM", lb.backingFile)
		}
		log.G(ctx).WithFields(logrus.Fields{
			"uvmPath":      lb.uvmPath,
			"refCount":     lb.refCount,
			"deviceNumber": lb.deviceNumber,
		}).Debug("removed loopback location")
		uvm.loopbackDevices[lb.deviceNumber] = nil
	} else {
		lb.refCount--
	}
	return nil
}
