//go:build linux
// +build linux

package pmem

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/guest/storage"
	dm "github.com/Microsoft/hcsshim/internal/guest/storage/devicemapper"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// Test dependencies
var (
	osMkdirAll                   = os.MkdirAll
	osRemoveAll                  = os.RemoveAll
	unixMount                    = unix.Mount
	mountInternal                = mount
	createZeroSectorLinearTarget = dm.CreateZeroSectorLinearTarget
	createVerityTarget           = dm.CreateVerityTarget
	removeDevice                 = dm.RemoveDevice
)

const (
	pMemFmt         = "/dev/pmem%d"
	linearDeviceFmt = "dm-linear-pmem%d-%d-%d"
	verityDeviceFmt = "dm-verity-pmem%d-%s"
)

// mount mounts source to target via unix.Mount
func mount(ctx context.Context, source, target string) (err error) {
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

// GetDevicePath returns VPMem device path
func GetDevicePath(devNumber uint32) string {
	return fmt.Sprintf(pMemFmt, devNumber)
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
func Mount(
	ctx context.Context,
	device uint32,
	target string,
	mappingInfo *guestresource.LCOWVPMemMappingInfo,
	verityInfo *guestresource.DeviceVerityInfo,
) (err error) {
	mCtx, span := otelutil.StartSpan(ctx, "pmem::Mount", trace.WithAttributes(
		attribute.Int64("deviceNumber", int64(device)),
		attribute.String("target", target)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	devicePath := GetDevicePath(device)

	// dm-linear target has to be created first. When verity info is also present, the linear target becomes the data
	// device instead of the original VPMem.
	if mappingInfo != nil {
		dmLinearName := fmt.Sprintf(linearDeviceFmt, device, mappingInfo.DeviceOffsetInBytes, mappingInfo.DeviceSizeInBytes)
		if devicePath, err = createZeroSectorLinearTarget(mCtx, devicePath, dmLinearName, mappingInfo); err != nil {
			return err
		}
		defer func() {
			if err != nil {
				if err := removeDevice(dmLinearName); err != nil {
					log.G(mCtx).WithError(err).Debugf("failed to cleanup linear target: %s", dmLinearName)
				}
			}
		}()
	}

	if verityInfo != nil {
		dmVerityName := fmt.Sprintf(verityDeviceFmt, device, verityInfo.RootDigest)
		if devicePath, err = createVerityTarget(mCtx, devicePath, dmVerityName, verityInfo); err != nil {
			return err
		}
		defer func() {
			if err != nil {
				if err := removeDevice(dmVerityName); err != nil {
					log.G(mCtx).WithError(err).Debugf("failed to cleanup verity target: %s", dmVerityName)
				}
			}
		}()
	}

	return mountInternal(mCtx, devicePath, target)
}

// Unmount unmounts `target` and removes corresponding linear and verity targets when needed
func Unmount(
	ctx context.Context,
	devNumber uint32,
	target string,
	mappingInfo *guestresource.LCOWVPMemMappingInfo,
	verityInfo *guestresource.DeviceVerityInfo,
) (err error) {
	_, span := otelutil.StartSpan(ctx, "pmem::Unmount", trace.WithAttributes(
		attribute.Int64("device", int64(devNumber)),
		attribute.String("target", target)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if err := storage.UnmountPath(ctx, target, true); err != nil {
		return errors.Wrapf(err, "failed to unmount target: %s", target)
	}

	if verityInfo != nil {
		dmVerityName := fmt.Sprintf(verityDeviceFmt, devNumber, verityInfo.RootDigest)
		if err := dm.RemoveDevice(dmVerityName); err != nil {
			// The target is already unmounted at this point, ignore potential errors
			log.G(ctx).WithError(err).Debugf("failed to remove dm verity target: %s", dmVerityName)
		}
	}

	if mappingInfo != nil {
		dmLinearName := fmt.Sprintf(linearDeviceFmt, devNumber, mappingInfo.DeviceOffsetInBytes, mappingInfo.DeviceSizeInBytes)
		if err := dm.RemoveDevice(dmLinearName); err != nil {
			// The target is already unmounted at this point, ignore potential errors
			log.G(ctx).WithError(err).Debugf("failed to remove dm linear target: %s", dmLinearName)
		}
	}

	return nil
}
