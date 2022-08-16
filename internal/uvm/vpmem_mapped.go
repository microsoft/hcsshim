//go:build windows

package uvm

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

const (
	PageSize             = 0x1000
	MaxMappedDeviceCount = 1024
)

const lcowPackedVPMemLayerFmt = "/run/layers/p%d-%d-%d"

type mappedDeviceInfo struct {
	vPMemInfoDefault
	mappedRegion memory.MappedRegion
	sizeInBytes  uint64
}

type vPMemInfoMulti struct {
	memory.PoolAllocator
	maxSize              uint64
	maxMappedDeviceCount uint32
	mappings             map[string]*mappedDeviceInfo
}

func newVPMemMappedDevice(hostPath, uvmPath string, sizeBytes uint64, memReg memory.MappedRegion) *mappedDeviceInfo {
	return &mappedDeviceInfo{
		vPMemInfoDefault: vPMemInfoDefault{
			hostPath: hostPath,
			uvmPath:  uvmPath,
			refCount: 1,
		},
		mappedRegion: memReg,
		sizeInBytes:  sizeBytes,
	}
}

func newPackedVPMemDevice() *vPMemInfoMulti {
	return &vPMemInfoMulti{
		PoolAllocator:        memory.NewPoolMemoryAllocator(),
		maxSize:              DefaultVPMemSizeBytes,
		mappings:             make(map[string]*mappedDeviceInfo),
		maxMappedDeviceCount: MaxMappedDeviceCount,
	}
}

func pageAlign(t uint64) uint64 {
	if t%PageSize == 0 {
		return t
	}
	return (t/PageSize + 1) * PageSize
}

// newMappedVPMemModifyRequest creates an hcsschema.ModifySettingsRequest to modify VPMem devices/mappings
// for the multi-mapping setup
func newMappedVPMemModifyRequest(
	ctx context.Context,
	rType guestrequest.RequestType,
	deviceNumber uint32,
	md *mappedDeviceInfo,
	uvm *UtilityVM,
) (*hcsschema.ModifySettingRequest, error) {
	guestSettings := guestresource.LCOWMappedVPMemDevice{
		DeviceNumber: deviceNumber,
		MountPath:    md.uvmPath,
		MappingInfo: &guestresource.LCOWVPMemMappingInfo{
			DeviceOffsetInBytes: md.mappedRegion.Offset(),
			DeviceSizeInBytes:   md.sizeInBytes,
		},
	}

	if verity, err := readVeritySuperBlock(ctx, md.hostPath); err != nil {
		log.G(ctx).WithError(err).WithField("hostPath", md.hostPath).Debug("unable to read dm-verity information from VHD")
	} else {
		log.G(ctx).WithFields(logrus.Fields{
			"hostPath":   md.hostPath,
			"rootDigest": verity.RootDigest,
		}).Debug("adding multi-mapped VPMem with dm-verity")
		guestSettings.VerityInfo = verity
	}

	request := &hcsschema.ModifySettingRequest{
		RequestType: rType,
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeVPMemDevice,
			RequestType:  rType,
			Settings:     guestSettings,
		},
	}

	pmem := uvm.vpmemDevicesMultiMapped[deviceNumber]
	switch rType {
	case guestrequest.RequestTypeAdd:
		if pmem == nil {
			request.Settings = hcsschema.VirtualPMemDevice{
				ReadOnly:    true,
				HostPath:    md.hostPath,
				ImageFormat: "Vhd1",
			}
			request.ResourcePath = fmt.Sprintf(resourcepaths.VPMemControllerResourceFormat, deviceNumber)
		} else {
			request.Settings = hcsschema.VirtualPMemMapping{
				HostPath:    md.hostPath,
				ImageFormat: "Vhd1",
			}
			request.ResourcePath = fmt.Sprintf(resourcepaths.VPMemDeviceResourceFormat, deviceNumber, md.mappedRegion.Offset())
		}
	case guestrequest.RequestTypeRemove:
		if pmem == nil {
			return nil, errors.Errorf("no device found at location %d", deviceNumber)
		}
		request.ResourcePath = fmt.Sprintf(resourcepaths.VPMemDeviceResourceFormat, deviceNumber, md.mappedRegion.Offset())
	default:
		return nil, errors.New("unsupported request type")
	}

	log.G(ctx).WithFields(logrus.Fields{
		"deviceNumber": deviceNumber,
		"hostPath":     md.hostPath,
		"uvmPath":      md.uvmPath,
	}).Debugf("new mapped VPMem modify request: %v", request)
	return request, nil
}

// mapVHDLayer adds `device` to mappings
func (pmem *vPMemInfoMulti) mapVHDLayer(ctx context.Context, device *mappedDeviceInfo) (err error) {
	if md, ok := pmem.mappings[device.hostPath]; ok {
		md.refCount++
		return nil
	}

	log.G(ctx).WithFields(logrus.Fields{
		"hostPath":     device.hostPath,
		"mountPath":    device.uvmPath,
		"deviceOffset": device.mappedRegion.Offset(),
		"deviceSize":   device.sizeInBytes,
	}).Debug("mapped new device")

	pmem.mappings[device.hostPath] = device
	return nil
}

// unmapVHDLayer removes mapped device with `hostPath` from mappings and releases allocated memory
func (pmem *vPMemInfoMulti) unmapVHDLayer(ctx context.Context, hostPath string) (err error) {
	dev, ok := pmem.mappings[hostPath]
	if !ok {
		return ErrNotAttached
	}

	if dev.refCount > 1 {
		dev.refCount--
		return nil
	}

	if err := pmem.Release(dev.mappedRegion); err != nil {
		return err
	}
	log.G(ctx).WithFields(logrus.Fields{
		"hostPath": dev.hostPath,
	}).Debugf("Done releasing resources: %s", dev.hostPath)
	delete(pmem.mappings, hostPath)
	return nil
}

// findVPMemMappedDevice finds a VHD device that's been mapped on VPMem surface
func (uvm *UtilityVM) findVPMemMappedDevice(ctx context.Context, hostPath string) (uint32, *mappedDeviceInfo, error) {
	for i := uint32(0); i < uvm.vpmemMaxCount; i++ {
		vi := uvm.vpmemDevicesMultiMapped[i]
		if vi != nil {
			if vhd, ok := vi.mappings[hostPath]; ok {
				log.G(ctx).WithFields(logrus.Fields{
					"deviceNumber": i,
					"hostPath":     hostPath,
					"uvmPath":      vhd.uvmPath,
					"refCount":     vhd.refCount,
					"deviceSize":   vhd.sizeInBytes,
					"deviceOffset": vhd.mappedRegion.Offset(),
				}).Debug("found mapped VHD")
				return i, vhd, nil
			}
		}
	}
	return 0, nil, ErrNotAttached
}

// allocateNextVPMemMappedDeviceLocation allocates a memory region with a minimum offset on the VPMem surface,
// where the device with a given `devSize` can be mapped.
func (uvm *UtilityVM) allocateNextVPMemMappedDeviceLocation(ctx context.Context, devSize uint64) (uint32, memory.MappedRegion, error) {
	// device size has to be page aligned
	devSize = pageAlign(devSize)

	for i := uint32(0); i < uvm.vpmemMaxCount; i++ {
		pmem := uvm.vpmemDevicesMultiMapped[i]
		if pmem == nil {
			pmem = newPackedVPMemDevice()
			uvm.vpmemDevicesMultiMapped[i] = pmem
		}

		if len(pmem.mappings) >= int(pmem.maxMappedDeviceCount) {
			continue
		}

		reg, err := pmem.Allocate(devSize)
		if err != nil {
			continue
		}
		log.G(ctx).WithFields(logrus.Fields{
			"deviceNumber": i,
			"deviceOffset": reg.Offset(),
			"deviceSize":   devSize,
		}).Debug("found offset for mapped VHD on an existing VPMem device")
		return i, reg, nil
	}
	return 0, nil, ErrNoAvailableLocation
}

// addVPMemMappedDevice adds container layer as a mapped device, first mapped device is added as a regular
// VPMem device, but subsequent additions will call into mapping APIs
//
// Lock MUST be held when calling this function
func (uvm *UtilityVM) addVPMemMappedDevice(ctx context.Context, hostPath string) (_ string, err error) {
	if _, dev, err := uvm.findVPMemMappedDevice(ctx, hostPath); err == nil {
		dev.refCount++
		return dev.uvmPath, nil
	}

	st, err := os.Stat(hostPath)
	if err != nil {
		return "", err
	}
	// NOTE: On the guest side devSize is used to create a device mapper linear target, which is then used to create
	// device mapper verity target. Since the dm-verity hash device is appended after ext4 data, we need the full size
	// on disk (minus VHD footer), otherwise the resulting linear target will have hash device truncated and verity
	// target creation will fail as a result.
	devSize := pageAlign(uint64(st.Size()))
	deviceNumber, memReg, err := uvm.allocateNextVPMemMappedDeviceLocation(ctx, devSize)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			pmem := uvm.vpmemDevicesMultiMapped[deviceNumber]
			if err := pmem.Release(memReg); err != nil {
				log.G(ctx).WithError(err).Debugf("failed to reclaim pmem region: %s", err)
			}
		}
	}()

	uvmPath := fmt.Sprintf(lcowPackedVPMemLayerFmt, deviceNumber, memReg.Offset(), devSize)
	md := newVPMemMappedDevice(hostPath, uvmPath, devSize, memReg)
	modification, err := newMappedVPMemModifyRequest(ctx, guestrequest.RequestTypeAdd, deviceNumber, md, uvm)
	if err := uvm.modify(ctx, modification); err != nil {
		return "", errors.Errorf("uvm::addVPMemMappedDevice: failed to modify utility VM configuration: %s", err)
	}
	defer func() {
		if err != nil {
			rmRequest, _ := newMappedVPMemModifyRequest(ctx, guestrequest.RequestTypeRemove, deviceNumber, md, uvm)
			if err := uvm.modify(ctx, rmRequest); err != nil {
				log.G(ctx).WithError(err).Debugf("failed to rollback modification")
			}
		}
	}()

	pmem := uvm.vpmemDevicesMultiMapped[deviceNumber]
	if err := pmem.mapVHDLayer(ctx, md); err != nil {
		return "", errors.Wrapf(err, "failed to update internal state")
	}
	return uvmPath, nil
}

// removeVPMemMappedDevice removes a mapped container layer. The VPMem device itself
// is never removed after being added.
//
// The bug occurs when we try to clean up a mapped VHD at non-zero offset.
// What happens is, the device-mapper target that corresponds to the mapped
// layer VHD is unmounted and removed inside the guest, however removal on
// the host through HCS API fails with "not found", because HCS API doesn't
// allow removal of a VPMem if it doesn't have a mapped device at offset 0, this
// results in the reference not being decreased and hcsshim "thinking" that the
// layer is still present. Next time this layer is used by a different
// container, it appears as if the layer is still mounted inside the guest as
// well, which results in overlayfs mount failures.
//
// Lock MUST be held when calling this function
func (uvm *UtilityVM) removeVPMemMappedDevice(ctx context.Context, hostPath string) error {
	devNum, md, err := uvm.findVPMemMappedDevice(ctx, hostPath)
	if err != nil {
		return err
	}
	if md.refCount > 1 {
		md.refCount--
		return nil
	}

	modification, err := newMappedVPMemModifyRequest(ctx, guestrequest.RequestTypeRemove, devNum, md, uvm)
	if err != nil {
		return err
	}

	if err := uvm.modify(ctx, modification); err != nil {
		return errors.Errorf("failed to remove packed VPMem %s from UVM %s: %s", md.hostPath, uvm.id, err)
	}

	pmem := uvm.vpmemDevicesMultiMapped[devNum]
	if err := pmem.unmapVHDLayer(ctx, hostPath); err != nil {
		log.G(ctx).WithError(err).Debugf("failed unmapping VHD layer %s", hostPath)
	}
	if len(pmem.mappings) == 0 {
		uvm.vpmemDevicesMultiMapped[devNum] = nil
	}
	return nil
}
