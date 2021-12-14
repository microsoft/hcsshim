package uvm

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/ext4/dmverity"
	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
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

// fileSystemSize retrieves ext4 fs SuperBlock and returns the file system size and block size
func fileSystemSize(vhdPath string) (int64, int, error) {
	sb, err := tar2ext4.ReadExt4SuperBlock(vhdPath)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to read ext4 super block")
	}
	blockSize := 1024 * (1 << sb.LogBlockSize)
	fsSize := int64(blockSize) * int64(sb.BlocksCountLow)
	return fsSize, blockSize, nil
}

// readVeritySuperBlock reads ext4 super block for a given VHD to then further read the dm-verity super block
// and root hash
func readVeritySuperBlock(ctx context.Context, layerPath string) (*guestresource.DeviceVerityInfo, error) {
	// dm-verity information is expected to be appended, the size of ext4 data will be the offset
	// of the dm-verity super block, followed by merkle hash tree
	ext4SizeInBytes, ext4BlockSize, err := fileSystemSize(layerPath)
	if err != nil {
		return nil, err
	}

	dmvsb, err := dmverity.ReadDMVerityInfo(layerPath, ext4SizeInBytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read dm-verity super block")
	}
	log.G(ctx).WithFields(logrus.Fields{
		"layerPath":     layerPath,
		"rootHash":      dmvsb.RootDigest,
		"algorithm":     dmvsb.Algorithm,
		"salt":          dmvsb.Salt,
		"dataBlocks":    dmvsb.DataBlocks,
		"dataBlockSize": dmvsb.DataBlockSize,
	}).Debug("dm-verity information")

	return &guestresource.DeviceVerityInfo{
		Ext4SizeInBytes: ext4SizeInBytes,
		BlockSize:       ext4BlockSize,
		RootDigest:      dmvsb.RootDigest,
		Algorithm:       dmvsb.Algorithm,
		Salt:            dmvsb.Salt,
		Version:         int(dmvsb.Version),
		SuperBlock:      true,
	}, nil
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
		RequestType: guestrequest.RequestTypeAdd,
		Settings: hcsschema.VirtualPMemDevice{
			HostPath:    hostPath,
			ReadOnly:    true,
			ImageFormat: "Vhd1",
		},
		ResourcePath: fmt.Sprintf(resourcepaths.VPMemControllerResourceFormat, deviceNumber),
	}

	uvmPath := fmt.Sprintf(lcowDefaultVPMemLayerFmt, deviceNumber)
	guestSettings := guestresource.LCOWMappedVPMemDevice{
		DeviceNumber: deviceNumber,
		MountPath:    uvmPath,
	}
	if v, iErr := readVeritySuperBlock(ctx, hostPath); iErr != nil {
		log.G(ctx).WithError(iErr).WithField("hostPath", hostPath).Debug("unable to read dm-verity information from VHD")
	} else {
		if v != nil {
			log.G(ctx).WithFields(logrus.Fields{
				"hostPath":   hostPath,
				"rootDigest": v.RootDigest,
			}).Debug("adding VPMem with dm-verity")
		}
		guestSettings.VerityInfo = v
	}

	modification.GuestRequest = guestrequest.ModificationRequest{
		ResourceType: guestresource.ResourceTypeVPMemDevice,
		RequestType:  guestrequest.RequestTypeAdd,
		Settings:     guestSettings,
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

	var verity *guestresource.DeviceVerityInfo
	if v, _ := readVeritySuperBlock(ctx, hostPath); v != nil {
		log.G(ctx).WithFields(logrus.Fields{
			"hostPath":   hostPath,
			"rootDigest": v.RootDigest,
		}).Debug("removing VPMem with dm-verity")
		verity = v
	}
	modification := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeRemove,
		ResourcePath: fmt.Sprintf(resourcepaths.VPMemControllerResourceFormat, deviceNumber),
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeVPMemDevice,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings: guestresource.LCOWMappedVPMemDevice{
				DeviceNumber: deviceNumber,
				MountPath:    device.uvmPath,
				VerityInfo:   verity,
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
