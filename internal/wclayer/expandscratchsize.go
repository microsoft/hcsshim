package wclayer

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/Microsoft/go-winio/vhd"
	"github.com/Microsoft/hcsshim/internal/hcserror"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/osversion"
	"go.opencensus.io/trace"
)

// ExpandScratchSize expands the size of a layer to at least size bytes.
func ExpandScratchSize(ctx context.Context, path string, size uint64) (err error) {
	title := "hcsshim::ExpandScratchSize"
	ctx, span := trace.StartSpan(ctx, title)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("path", path),
		trace.Int64Attribute("size", int64(size)))

	err = expandSandboxSize(&stdDriverInfo, path, size)
	if err != nil {
		return hcserror.New(err, title, "")
	}

	// Manually expand the volume now in order to work around bugs in 19H1 and
	// prerelease versions of Vb. Remove once this is fixed in Windows.
	if build := osversion.Build(); build >= osversion.V19H1 && build < 19020 {
		err = expandSandboxVolume(ctx, path)
		if err != nil {
			return err
		}
	}
	return nil
}

func expandSandboxVolume(ctx context.Context, path string) error {
	// Mount the sandbox VHD temporarily.
	vhdPath := filepath.Join(path, "sandbox.vhdx")
	vhdHandle, err := vhd.OpenVirtualDisk(vhdPath, vhd.VirtualDiskAccessNone, vhd.OpenVirtualDiskFlagNone)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(vhdHandle)

	err = vhd.AttachVirtualDisk(vhdHandle, vhd.AttachVirtualDiskFlagNone, &vhd.AttachVirtualDiskParameters{Version: 2})
	if err != nil {
		return err
	}

	// Open the volume.
	volumePath, err := GetLayerMountPath(ctx, path)
	if err != nil {
		return err
	}
	if volumePath[len(volumePath)-1] == '\\' {
		volumePath = volumePath[:len(volumePath)-1]
	}
	volume, err := os.OpenFile(volumePath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer volume.Close()

	// Get the volume's underlying partition size in NTFS clusters.
	var (
		partitionSize int64
		bytes         uint32
	)
	const _IOCTL_DISK_GET_LENGTH_INFO = 0x0007405C
	err = syscall.DeviceIoControl(syscall.Handle(volume.Fd()), _IOCTL_DISK_GET_LENGTH_INFO, nil, 0, (*byte)(unsafe.Pointer(&partitionSize)), 8, &bytes, nil)
	if err != nil {
		return &os.PathError{Op: "IOCTL_DISK_GET_LENGTH_INFO", Path: volume.Name(), Err: err}
	}
	const (
		clusterSize = 4096
		sectorSize  = 512
	)
	targetClusters := partitionSize / clusterSize

	// Get the volume's current size in NTFS clusters.
	var volumeSize int64
	err = getDiskFreeSpaceEx(volume.Name()+"\\", nil, &volumeSize, nil)
	if err != nil {
		return &os.PathError{Op: "GetDiskFreeSpaceEx", Path: volume.Name(), Err: err}
	}
	volumeClusters := volumeSize / clusterSize

	// Only resize the volume if there is space to grow, otherwise this will
	// fail with invalid parameter. NTFS reserves one cluster.
	if volumeClusters+1 < targetClusters {
		targetSectors := targetClusters * (clusterSize / sectorSize)
		const _FSCTL_EXTEND_VOLUME = 0x000900F0
		err = syscall.DeviceIoControl(syscall.Handle(volume.Fd()), _FSCTL_EXTEND_VOLUME, (*byte)(unsafe.Pointer(&targetSectors)), 8, nil, 0, &bytes, nil)
		if err != nil {
			return &os.PathError{Op: "FSCTL_EXTEND_VOLUME", Path: volume.Name(), Err: err}
		}
	}
	return nil
}
