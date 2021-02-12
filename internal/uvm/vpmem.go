package uvm

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/requesttype"
)

const (
	lcowDefaultVPMemLayerFmt = "/run/layers/p%d"
)

var (
	// ErrMaxVPMemLayerSize is the error returned when the size of `hostPath` is
	// greater than the max vPMem layer size set at create time.
	ErrMaxVPMemLayerSize = errors.New("layer size is to large for VPMEM max size")
)

type vPMemInfoDefault struct {
	hostPath string
	uvmPath  string
	refCount uint32
}

func newDefaultVPMemInfo(hostPath, uvmPath string) *vPMemInfoDefault {
	return &vPMemInfoDefault{
		hostPath: hostPath,
		uvmPath:  uvmPath,
		refCount: 1,
	}
}

// findNextVPMemSlot finds next available VPMem slot.
//
// Lock MUST be held when calling this function.
func (uvm *UtilityVM) findNextVPMemSlot(ctx context.Context, hostPath string) (uint32, error) {
	for i := uint32(0); i < uvm.vpmemMaxCount; i++ {
		if uvm.vpmemDevicesDefault[i] == nil {
			log.G(ctx).WithFields(logrus.Fields{
				"hostPath":     hostPath,
				"deviceNumber": i,
			}).Debug("allocated VPMem location")
			return i, nil
		}
	}
	return 0, ErrNoAvailableLocation
}

// findVPMemSlot looks up `findThisHostPath` in already mounted VPMem devices
//
// Lock MUST be held when calling this function
func (uvm *UtilityVM) findVPMemSlot(ctx context.Context, findThisHostPath string) (uint32, error) {
	for i := uint32(0); i < uvm.vpmemMaxCount; i++ {
		if vi := uvm.vpmemDevicesDefault[i]; vi != nil && vi.hostPath == findThisHostPath {
			log.G(ctx).WithFields(logrus.Fields{
				"hostPath":     vi.hostPath,
				"uvmPath":      vi.uvmPath,
				"refCount":     vi.refCount,
				"deviceNumber": i,
			}).Debug("found VPMem location")
			return i, nil
		}
	}
	return 0, ErrNotAttached
}

// addVPMemDefault adds a VPMem disk to a utility VM at the next available location and
// returns the UVM path where the layer was mounted.
func (uvm *UtilityVM) addVPMemDefault(ctx context.Context, hostPath string) (_ string, err error) {
	if devNumber, err := uvm.findVPMemSlot(ctx, hostPath); err == nil {
		device := uvm.vpmemDevicesDefault[devNumber]
		device.refCount++
		return device.uvmPath, nil
	}

	fi, err := os.Stat(hostPath)
	if err != nil {
		return "", err
	}
	if uint64(fi.Size()) > uvm.vpmemMaxSizeBytes {
		return "", ErrMaxVPMemLayerSize
	}

	deviceNumber, err := uvm.findNextVPMemSlot(ctx, hostPath)
	if err != nil {
		return "", err
	}
	modification := &hcsschema.ModifySettingRequest{
		RequestType: requesttype.Add,
		Settings: hcsschema.VirtualPMemDevice{
			HostPath:    hostPath,
			ReadOnly:    true,
			ImageFormat: "Vhd1",
		},
		ResourcePath: fmt.Sprintf(resourcepaths.VPMemControllerResourceFormat, deviceNumber),
	}
	uvmPath := fmt.Sprintf(lcowDefaultVPMemLayerFmt, deviceNumber)
	modification.GuestRequest = guestrequest.GuestRequest{
		ResourceType: guestrequest.ResourceTypeVPMemDevice,
		RequestType:  requesttype.Add,
		Settings: guestrequest.LCOWMappedVPMemDevice{
			DeviceNumber: deviceNumber,
			MountPath:    uvmPath,
		},
	}

	if err := uvm.modify(ctx, modification); err != nil {
		return "", errors.Errorf("uvm::addVPMemDefault: failed to modify utility VM configuration: %s", err)
	}

	uvm.vpmemDevicesDefault[deviceNumber] = newDefaultVPMemInfo(hostPath, uvmPath)
	return uvmPath, nil
}

// removeVPMemDefault removes a VPMem disk from a Utility VM. If the `hostPath` is not
// attached returns `ErrNotAttached`.
func (uvm *UtilityVM) removeVPMemDefault(ctx context.Context, hostPath string) error {
	deviceNumber, err := uvm.findVPMemSlot(ctx, hostPath)
	if err != nil {
		return err
	}

	device := uvm.vpmemDevicesDefault[deviceNumber]
	if device.refCount > 1 {
		device.refCount--
		return nil
	}

	modification := &hcsschema.ModifySettingRequest{
		RequestType:  requesttype.Remove,
		ResourcePath: fmt.Sprintf(resourcepaths.VPMemControllerResourceFormat, deviceNumber),
		GuestRequest: guestrequest.GuestRequest{
			ResourceType: guestrequest.ResourceTypeVPMemDevice,
			RequestType:  requesttype.Remove,
			Settings: guestrequest.LCOWMappedVPMemDevice{
				DeviceNumber: deviceNumber,
				MountPath:    device.uvmPath,
			},
		},
	}
	if err := uvm.modify(ctx, modification); err != nil {
		return errors.Errorf("failed to remove VPMEM %s from utility VM %s: %s", hostPath, uvm.id, err)
	}
	log.G(ctx).WithFields(logrus.Fields{
		"hostPath":     device.hostPath,
		"uvmPath":      device.uvmPath,
		"refCount":     device.refCount,
		"deviceNumber": deviceNumber,
	}).Debug("removed VPMEM location")

	uvm.vpmemDevicesDefault[deviceNumber] = nil

	return nil
}

func (uvm *UtilityVM) AddVPMem(ctx context.Context, hostPath string) (string, error) {
	if uvm.operatingSystem != "linux" {
		return "", errNotSupported
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	if uvm.vpmemMultiMapping {
		return uvm.addVPMemMappedDevice(ctx, hostPath)
	}
	return uvm.addVPMemDefault(ctx, hostPath)
}

func (uvm *UtilityVM) RemoveVPMem(ctx context.Context, hostPath string) error {
	if uvm.operatingSystem != "linux" {
		return errNotSupported
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	if uvm.vpmemMultiMapping {
		return uvm.removeVPMemMappedDevice(ctx, hostPath)
	}
	return uvm.removeVPMemDefault(ctx, hostPath)
}
