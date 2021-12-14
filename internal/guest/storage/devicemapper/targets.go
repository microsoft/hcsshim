//go:build linux
// +build linux

package devicemapper

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"go.opencensus.io/trace"

	"github.com/Microsoft/hcsshim/ext4/dmverity"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// CreateZeroSectorLinearTarget creates dm-linear target for a device at `devPath` and `mappingInfo`, returns
// virtual block device path.
func CreateZeroSectorLinearTarget(ctx context.Context, devPath, devName string, mappingInfo *guestresource.LCOWVPMemMappingInfo) (_ string, err error) {
	_, span := trace.StartSpan(ctx, "devicemapper::CreateZeroSectorLinearTarget")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	size := int64(mappingInfo.DeviceSizeInBytes)
	offset := int64(mappingInfo.DeviceOffsetInBytes)
	linearTarget := zeroSectorLinearTarget(size, devPath, offset)

	span.AddAttributes(
		trace.StringAttribute("devicePath", devPath),
		trace.Int64Attribute("deviceStart", offset),
		trace.Int64Attribute("sectorSize", size),
		trace.StringAttribute("linearTable", fmt.Sprintf("%s: '%d %d %s'", devName, linearTarget.SectorStart, linearTarget.LengthInBlocks, linearTarget.Params)))

	devMapperPath, err := CreateDevice(devName, CreateReadOnly, []Target{linearTarget})
	if err != nil {
		return "", errors.Wrapf(err, "failed to create dm-linear target, device=%s, offset=%d", devPath, mappingInfo.DeviceOffsetInBytes)
	}

	return devMapperPath, nil
}

// CreateVerityTarget creates a dm-verity target for a given device and returns created virtual block device path.
//
// Example verity target table:
// 0 417792 verity 1 /dev/sdb /dev/sdc 4096 4096 52224 1 sha256 2aa4f7b7b6...f4952060e8 762307f4bc8...d2a6b7595d8..
// |    |     |    |     |     |        |    |    |    |    |              |                        |
// start|     |    |  data_dev |  data_block | #blocks | hash_alg      root_digest                salt
//     size   |  version    hash_dev         |     hash_offset
//          target                       hash_block
func CreateVerityTarget(ctx context.Context, devPath, devName string, verityInfo *guestresource.DeviceVerityInfo) (_ string, err error) {
	_, span := trace.StartSpan(ctx, "devicemapper::CreateVerityTarget")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	dmBlocks := verityInfo.Ext4SizeInBytes / blockSize
	dataBlocks := verityInfo.Ext4SizeInBytes / int64(verityInfo.BlockSize)
	hashOffsetBlocks := dataBlocks
	if verityInfo.SuperBlock {
		hashOffsetBlocks++
	}
	hashes := fmt.Sprintf("%s %s %s", verityInfo.Algorithm, verityInfo.RootDigest, verityInfo.Salt)
	blkInfo := fmt.Sprintf("%d %d %d %d", verityInfo.BlockSize, verityInfo.BlockSize, dataBlocks, hashOffsetBlocks)
	devices := fmt.Sprintf("%s %s", devPath, devPath)

	verityTarget := Target{
		SectorStart:    0,
		LengthInBlocks: dmBlocks,
		Type:           dmverity.VeritySignature,
		Params:         fmt.Sprintf("%d %s %s %s", verityInfo.Version, devices, blkInfo, hashes),
	}

	span.AddAttributes(
		trace.StringAttribute("devicePath", devPath),
		trace.Int64Attribute("sectorSize", dmBlocks),
		trace.StringAttribute("verityTable", verityTarget.Params))

	mapperPath, err := CreateDevice(devName, CreateReadOnly, []Target{verityTarget})
	if err != nil {
		return "", errors.Wrapf(err, "failed to create dm-verity target. device=%s", devPath)
	}

	return mapperPath, nil
}
