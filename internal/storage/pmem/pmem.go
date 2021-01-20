// +build linux

package pmem

import (
	"context"
	"fmt"
	"os"

	"github.com/Microsoft/opengcs/internal/oc"
	"github.com/Microsoft/opengcs/internal/storage"
	dm "github.com/Microsoft/opengcs/internal/storage/devicemapper"
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
)

func mountInternal(source, target string) (err error) {
	if err := osMkdirAll(target, 0700); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			osRemoveAll(target)
		}
	}()

	flags := uintptr(unix.MS_RDONLY)
	if err := unixMount(source, target, "ext4", flags, "noload"); err != nil {
		return errors.Wrapf(err, "failed to mount %s onto %s", source, target)
	}
	return nil
}

// Mount mounts the pmem device at `/dev/pmem<device>` to `target`.
//
// `target` will be created. On mount failure the created `target` will be
// automatically cleaned up.
//
// Note: For now the platform only supports readonly pmem that is assumed to be
// `ext4`.
func Mount(ctx context.Context, device uint32, target string) (err error) {
	_, span := trace.StartSpan(ctx, "pmem::Mount")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.Int64Attribute("device", int64(device)),
		trace.StringAttribute("target", target))

	source := fmt.Sprintf(pMemFmt, device)
	return mountInternal(source, target)
}

// MountDM creates dm-linear block device and mounts it on `target`
func MountDM(ctx context.Context, device uint32, deviceStart, deviceSize int64, target string) (err error) {
	_, span := trace.StartSpan(ctx, "pmem::MountDM")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	lt := dm.PMemLinearTarget(deviceSize, fmt.Sprintf(pMemFmt, device), deviceStart)
	linearDeviceName := fmt.Sprintf(linearDeviceFmt, device, deviceStart, deviceSize)

	span.AddAttributes(
		trace.Int64Attribute("device", int64(device)),
		trace.Int64Attribute("deviceStart", deviceStart),
		trace.Int64Attribute("sectorSize", deviceSize),
		trace.StringAttribute("target", target),
		trace.StringAttribute("table", fmt.Sprintf("%s: '%d %d %s'", linearDeviceName, lt.SectorStart, lt.Length, lt.Params)))

	dmpath, err := dm.CreateDevice(linearDeviceName, dm.CreateReadOnly, []dm.Target{lt})
	if err != nil {
		return errors.Wrapf(err, "failed to create dm-linear target: pmem device: %d, offset: %d", device, deviceStart)
	}

	return mountInternal(dmpath, target)
}

// UnmountDM unmounts `target` and removes associated dm-linear block device
func UnmountDM(ctx context.Context, deviceNumber uint32, deviceStart, deviceSize int64, target string) (err error) {
	_, span := trace.StartSpan(ctx, "pmem::UnmountDM")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.Int64Attribute("device", int64(deviceNumber)),
		trace.Int64Attribute("deviceStart", deviceStart),
		trace.Int64Attribute("deviceSize", deviceSize),
		trace.StringAttribute("target", target))

	if err := storage.UnmountPath(ctx, target, true); err != nil {
		return err
	}

	linearDeviceName := fmt.Sprintf(linearDeviceFmt, deviceNumber, deviceStart, deviceSize)
	if err := dm.RemoveDevice(linearDeviceName); err != nil {
		return errors.Wrapf(err, "failed to remove dm-linear target %s", linearDeviceName)
	}
	return nil
}
