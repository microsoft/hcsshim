// +build linux

package pmem

import (
	"context"
	"fmt"
	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/log"
	"os"

	"github.com/Microsoft/hcsshim/internal/guest/storage"
	dm "github.com/Microsoft/hcsshim/internal/guest/storage/devicemapper"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sys/unix"
)

// Test dependencies
var (
	osMkdirAll  = os.MkdirAll
	osRemoveAll = os.RemoveAll
	unixMount   = unix.Mount
)

const (
	pMemFmt         = "/dev/pmem%d"
	linearDeviceFmt = "dm-linear-pmem%d-%d-%d"
	verityDeviceFmt = "dm-verity-pmem%d-%s"
)

// mountInternal mounts source to target via unix.Mount
func mountInternal(ctx context.Context, source, target string) (err error) {
	if err := osMkdirAll(target, 0700); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if err := osRemoveAll(target); err != nil {
				log.G(ctx).WithError(err).Debugf("error cleaning up target: %s", target)
			}
		}
	}()

	flags := uintptr(unix.MS_RDONLY)
	if err := unixMount(source, target, "ext4", flags, "noload"); err != nil {
		return errors.Wrapf(err, "failed to mount %s onto %s", source, target)
	}
	return nil
}

// Mount mounts the pmem device at `/dev/pmem<device>` to `target` in a basic scenario.
// If either mappingInfo or verityInfo are non-nil, the device-mapper framework is used
// to create linear and verity targets accordingly. If both are non-nil, the linear
// target is created first and used as the data/hash device for the verity target.
//
// `target` will be created. On mount failure the created `target` will be
// automatically cleaned up.
//
// Note: For now the platform only supports readonly pmem that is assumed to be
// `ext4`.
//
// Note: both mappingInfo and verityInfo can be non-nil at the same time, in that case
// linear target is created first and it becomes the data/hash device for verity target.
func Mount(ctx context.Context, device uint32, target string, mappingInfo *prot.DeviceMappingInfo, verityInfo *prot.DeviceVerityInfo) (err error) {
	mCtx, span := trace.StartSpan(ctx, "pmem::Mount")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.Int64Attribute("deviceNumber", int64(device)),
		trace.StringAttribute("target", target))

	devicePath := fmt.Sprintf(pMemFmt, device)
	// dm linear target has to be created first. when verity info is also present, the linear target becomes the data
	// device instead of the original VPMem.
	if mappingInfo != nil {
		dmLinearName := fmt.Sprintf(linearDeviceFmt, device, mappingInfo.DeviceOffsetInBytes, mappingInfo.DeviceSizeInBytes)
		if dmLinearPath, err := createDMLinearTarget(mCtx, devicePath, dmLinearName, target, mappingInfo); err != nil {
			return err
		} else {
			devicePath = dmLinearPath
		}
		defer func() {
			if err != nil {
				if err := dm.RemoveDevice(dmLinearName); err != nil {
					log.G(mCtx).WithError(err).Debugf("failed to cleanup linear target: %s", dmLinearName)
				}
			}
		}()
	}

	if verityInfo != nil {
		dmVerityName := fmt.Sprintf(verityDeviceFmt, device, verityInfo.RootDigest)
		if dmVerityPath, err := createDMVerityTarget(mCtx, devicePath, dmVerityName, target, verityInfo); err != nil {
			return err
		} else {
			devicePath = dmVerityPath
		}
		defer func() {
			if err != nil {
				if err := dm.RemoveDevice(dmVerityName); err != nil {
					log.G(mCtx).WithError(err).Debugf("failed to cleanup verity target: %s", dmVerityName)
				}
			}
		}()
	}

	return mountInternal(mCtx, devicePath, target)
}

// createDMLinearTarget creates dm-linear target from a given `device` slot location and `mappingInfo`
func createDMLinearTarget(ctx context.Context, devPath, devName string, target string, mappingInfo *prot.DeviceMappingInfo) (_ string, err error) {
	_, span := trace.StartSpan(ctx, "pmem::createDMLinearTarget")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	linearTarget := dm.PMemLinearTarget(mappingInfo.DeviceSizeInBytes, devPath, mappingInfo.DeviceOffsetInBytes)

	span.AddAttributes(
		trace.StringAttribute("devicePath", devPath),
		trace.Int64Attribute("deviceStart", mappingInfo.DeviceOffsetInBytes),
		trace.Int64Attribute("sectorSize", mappingInfo.DeviceSizeInBytes),
		trace.StringAttribute("target", target),
		trace.StringAttribute("linearTable", fmt.Sprintf("%s: '%d %d %s'", devName, linearTarget.SectorStart, linearTarget.LengthInBlocks, linearTarget.Params)))

	devMapperPath, err := dm.CreateDevice(devName, dm.CreateReadOnly, []dm.Target{linearTarget})
	if err != nil {
		return "", errors.Wrapf(err, "failed to create dm-linear target: pmem device: %s, offset: %d", devPath, mappingInfo.DeviceOffsetInBytes)
	}

	return devMapperPath, nil
}

// createDMVerityTarget creates a dm-verity target for a given device and mounts that target instead of the device itself
//
// verity target table
// 0 417792 verity 1 /dev/sdb /dev/sdc 4096 4096 52224 1 sha256 2aa4f7b7b6...f4952060e8 762307f4bc8...d2a6b7595d8..
// |    |     |    |     |     |        |    |    |    |    |              |                        |
// start|     |    |  data_dev |  data_block | #blocks | hash_alg      root_digest                salt
//     size   |  version    hash_dev         |     hash_offset
//          target                       hash_block
func createDMVerityTarget(ctx context.Context, devPath, devName, target string, verityInfo *prot.DeviceVerityInfo) (_ string, err error) {
	_, span := trace.StartSpan(ctx, "pmem::createDMVerityTarget")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	dmBlocks := verityInfo.Ext4SizeInBytes / dm.BlockSize
	dataBlocks := verityInfo.Ext4SizeInBytes / int64(verityInfo.BlockSize)
	hashOffsetBlocks := dataBlocks
	if verityInfo.SuperBlock {
		hashOffsetBlocks++
	}
	hashes := fmt.Sprintf("%s %s %s", verityInfo.Algorithm, verityInfo.RootDigest, verityInfo.Salt)
	blkInfo := fmt.Sprintf("%d %d %d %d", verityInfo.BlockSize, verityInfo.BlockSize, dataBlocks, hashOffsetBlocks)
	devices := fmt.Sprintf("%s %s", devPath, devPath)

	verityTarget := dm.Target{
		SectorStart:    0,
		LengthInBlocks: dmBlocks,
		Type:           "verity",
		Params:         fmt.Sprintf("%d %s %s %s", verityInfo.Version, devices, blkInfo, hashes),
	}

	span.AddAttributes(
		trace.StringAttribute("devicePath", devPath),
		trace.StringAttribute("target", target),
		trace.Int64Attribute("sectorSize", dmBlocks),
		trace.StringAttribute("verityTable", verityTarget.Params))

	mapperPath, err := dm.CreateDevice(devName, dm.CreateReadOnly, []dm.Target{verityTarget})
	if err != nil {
		return "", errors.Wrapf(err, "failed to create dm-verity target: pmem device: %s", devPath)
	}

	return mapperPath, nil
}

// Unmount unmounts `target` and removes corresponding linear and verity targets when needed
func Unmount(ctx context.Context, devNumber uint32, target string, mappingInfo *prot.DeviceMappingInfo, verityInfo *prot.DeviceVerityInfo) (err error) {
	_, span := trace.StartSpan(ctx, "pmem::Unmount")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.Int64Attribute("device", int64(devNumber)),
		trace.StringAttribute("target", target))

	if err := storage.UnmountPath(ctx, target, true); err != nil {
		return errors.Wrapf(err, "failed to unmount target: %s", target)
	}

	if verityInfo != nil {
		dmVerityName := fmt.Sprintf(verityDeviceFmt, devNumber, verityInfo.RootDigest)
		if err := dm.RemoveDevice(dmVerityName); err != nil {
			return errors.Wrapf(err, "failed to remove dm verity target: %s", dmVerityName)
		}
	}

	if mappingInfo != nil {
		dmLinearName := fmt.Sprintf(linearDeviceFmt, devNumber, mappingInfo.DeviceOffsetInBytes, mappingInfo.DeviceSizeInBytes)
		if err := dm.RemoveDevice(dmLinearName); err != nil {
			return errors.Wrapf(err, "failed to remove dm linear target: %s", dmLinearName)
		}
	}

	return nil
}
