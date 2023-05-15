package verity

import (
	"context"

	"github.com/Microsoft/hcsshim/ext4/dmverity"
	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

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

// ReadVeritySuperBlock reads ext4 super block for a given VHD to then further read the dm-verity super block
// and root hash
func ReadVeritySuperBlock(ctx context.Context, layerPath string) (*guestresource.DeviceVerityInfo, error) {
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
