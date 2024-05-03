//go:build linux
// +build linux

package scsi

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/Microsoft/hcsshim/internal/guest/storage"
	"github.com/Microsoft/hcsshim/internal/guest/storage/crypt"
	dm "github.com/Microsoft/hcsshim/internal/guest/storage/devicemapper"
	"github.com/Microsoft/hcsshim/internal/guest/storage/ext4"
	"github.com/Microsoft/hcsshim/internal/guest/storage/xfs"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// Test dependencies.
var (
	osMkdirAll  = os.MkdirAll
	osRemoveAll = os.RemoveAll
	unixMount   = unix.Mount

	// mock functions for testing getDevicePath
	osReadDir = os.ReadDir
	osStat    = os.Stat

	// getDevicePath is stubbed to make testing `Mount` easier.
	getDevicePath = GetDevicePath
	// createVerityTarget is stubbed for unit testing `Mount`.
	createVerityTarget = dm.CreateVerityTarget
	// removeDevice is stubbed for unit testing `Mount`.
	removeDevice = dm.RemoveDevice
	// encryptDevice is stubbed for unit testing `mount`
	encryptDevice = crypt.EncryptDevice
	// cleanupCryptDevice is stubbed for unit testing `mount`
	cleanupCryptDevice = crypt.CleanupCryptDevice
	// getDeviceFsType is stubbed for unit testing `mount`
	_getDeviceFsType = getDeviceFsType
	// storageUnmountPath is stubbed for unit testing `unmount`
	storageUnmountPath = storage.UnmountPath
	// tar2ext4.IsDeviceExt4 is stubbed for unit testing `getDeviceFsType`
	_tar2ext4IsDeviceExt4 = tar2ext4.IsDeviceExt4
	// ext4Format is stubbed for unit testing the `EnsureFilesystem` flow
	// in `mount`
	ext4Format = ext4.Format
	// ext4Format is stubbed for unit testing the `EnsureFilesystem` and
	// `Encrypt` flow in `mount`
	xfsFormat = xfs.Format
)

const (
	scsiDevicesPath  = "/sys/bus/scsi/devices"
	vmbusDevicesPath = "/sys/bus/vmbus/devices"
	verityDeviceFmt  = "dm-verity-scsi-contr%d-lun%d-p%d-%s"
	cryptDeviceFmt   = "dm-crypt-scsi-contr%d-lun%d-p%d"
)

// ActualControllerNumber retrieves the actual controller number assigned to a SCSI controller
// with number `passedController`.
// When HCS creates the UVM it adds 4 SCSI controllers to the UVM but the 1st SCSI
// controller according to HCS can actually show up as 2nd, 3rd or 4th controller inside
// the UVM. So the i'th controller from HCS' perspective could actually be j'th controller
// inside the UVM. However, we can refer to the SCSI controllers with their GUIDs (that
// are hardcoded) and then using that GUID find out the SCSI controller number inside the
// guest. This function does exactly that.
func ActualControllerNumber(_ context.Context, passedController uint8) (uint8, error) {
	// find the controller number by looking for a file named host<N> (e.g host1, host3 etc.)
	// `N` is the controller number.
	// Full file path would be /sys/bus/vmbus/devices/<controller-guid>/host<N>.
	controllerDirPath := path.Join(vmbusDevicesPath, guestrequest.ScsiControllerGuids[passedController])
	entries, err := os.ReadDir(controllerDirPath)
	if err != nil {
		return 0, err
	}

	for _, entry := range entries {
		baseName := path.Base(entry.Name())
		if !strings.HasPrefix(baseName, "host") {
			continue
		}
		controllerStr := baseName[len("host"):]
		controllerNum, err := strconv.ParseUint(controllerStr, 10, 8)
		if err != nil {
			return 0, fmt.Errorf("failed to parse controller number from %s: %w", baseName, err)
		}
		return uint8(controllerNum), nil
	}
	return 0, fmt.Errorf("host<N> directory not found inside %s", controllerDirPath)
}

// Config represents options that are used as part of setup/cleanup before
// mounting or after unmounting a device. This does not include options
// that are sent to the mount or unmount calls.
type Config struct {
	Encrypted        bool
	VerityInfo       *guestresource.DeviceVerityInfo
	EnsureFilesystem bool
	Filesystem       string
}

// Mount creates a mount from the SCSI device on `controller` index `lun` to
// `target`
//
// `target` will be created. On mount failure the created `target` will be
// automatically cleaned up.
//
// If the config has `encrypted` is set to true, the SCSI device will be
// encrypted using dm-crypt.
func Mount(
	ctx context.Context,
	controller,
	lun uint8,
	partition uint64,
	target string,
	readonly bool,
	options []string,
	config *Config) (err error) {
	spnCtx, span := otelutil.StartSpan(ctx, "scsi::Mount", trace.WithAttributes(
		attribute.Int64("controller", int64(controller)),
		attribute.Int64("lun", int64(lun)),
		attribute.Int64("partition", int64(partition))))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	source, err := getDevicePath(spnCtx, controller, lun, partition)
	if err != nil {
		return err
	}

	if readonly {
		if config.VerityInfo != nil {
			deviceHash := config.VerityInfo.RootDigest
			dmVerityName := fmt.Sprintf(verityDeviceFmt, controller, lun, partition, deviceHash)
			if source, err = createVerityTarget(spnCtx, source, dmVerityName, config.VerityInfo); err != nil {
				return err
			}
			defer func() {
				if err != nil {
					if err := removeDevice(dmVerityName); err != nil {
						log.G(spnCtx).WithError(err).WithField("verityTarget", dmVerityName).Debug("failed to cleanup verity target")
					}
				}
			}()
		}
	}

	if err := osMkdirAll(target, 0700); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = osRemoveAll(target)
		}
	}()

	// we only care about readonly mount option when mounting the device
	var flags uintptr
	data := ""
	if readonly {
		flags |= unix.MS_RDONLY
		data = "noload"
	}

	var deviceFS string
	if config.Encrypted {
		cryptDeviceName := fmt.Sprintf(cryptDeviceFmt, controller, lun, partition)
		encryptedSource, err := encryptDevice(spnCtx, source, cryptDeviceName)
		if err != nil {
			// todo (maksiman): add better retry logic, similar to how SCSI device mounts are
			// retried on unix.ENOENT and unix.ENXIO. The retry should probably be on an
			// error message rather than actual error, because we shell-out to cryptsetup.
			time.Sleep(500 * time.Millisecond)
			if encryptedSource, err = encryptDevice(spnCtx, source, cryptDeviceName); err != nil {
				return fmt.Errorf("failed to mount encrypted device %s: %w", source, err)
			}
		}
		source = encryptedSource
	} else {
		// Get the filesystem that is already on the device (if any) and use that
		// as the mountType unless `Filesystem` was given.
		deviceFS, err = _getDeviceFsType(source)
		if err != nil {
			// TODO (ambarve): add better retry logic, SCSI mounts sometimes return ENONENT or
			// ENXIO error if we try to open those devices immediately after mount. retry after a
			// few milliseconds.
			log.G(ctx).WithError(err).Trace("get device filesystem failed, retrying in 500ms")
			time.Sleep(500 * time.Millisecond)
			if deviceFS, err = _getDeviceFsType(source); err != nil {
				if config.Filesystem == "" || !errors.Is(err, ErrUnknownFilesystem) {
					return fmt.Errorf("getting device's filesystem: %w", err)
				}
			}
		}
		log.G(ctx).WithField("filesystem", deviceFS).Debug("filesystem found on device")
	}

	mountType := deviceFS
	if config.Filesystem != "" {
		mountType = config.Filesystem
	}

	// if EnsureFilesystem is set, then we need to check if the device has the
	// correct filesystem configured on it. If it does not, format the device
	// with the corect filesystem. Right now, we only support formatting ext4
	// and xfs.
	if config.EnsureFilesystem {
		// compare the actual fs found on the device to the filesystem requested
		if deviceFS != config.Filesystem {
			// re-format device to the correct fs
			switch config.Filesystem {
			case "ext4":
				if err := ext4Format(ctx, source); err != nil {
					return fmt.Errorf("ext4 format: %w", err)
				}
			case "xfs":
				if err = xfsFormat(source); err != nil {
					return fmt.Errorf("xfs format: %w", err)
				}
			default:
				return fmt.Errorf("unsupported filesystem %s requested for device", config.Filesystem)
			}
		}
	}

	// device should already be present under /dev, so we should not get an error
	// unless the command has actually errored out
	if err := unixMount(source, target, mountType, flags, data); err != nil {
		return fmt.Errorf("mounting: %w", err)
	}

	// remount the target to account for propagation flags
	_, pgFlags, _ := storage.ParseMountOptions(options)
	if len(pgFlags) != 0 {
		for _, pg := range pgFlags {
			if err := unixMount(target, target, "", pg, ""); err != nil {
				return err
			}
		}
	}

	return nil
}

// Unmount SCSI device mounted at `target`. Cleanup associated dm-verity and
// dm-crypt devices when necessary.
func Unmount(
	ctx context.Context,
	controller,
	lun uint8,
	partition uint64,
	target string,
	config *Config,
) (err error) {
	ctx, span := otelutil.StartSpan(ctx, "scsi::Unmount", trace.WithAttributes(
		attribute.Int64("controller", int64(controller)),
		attribute.Int64("lun", int64(lun)),
		attribute.Int64("partition", int64(partition)),
		attribute.String("target", target)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	// unmount target
	if err := storageUnmountPath(ctx, target, true); err != nil {
		return errors.Wrapf(err, "unmount failed: %s", target)
	}

	if config.VerityInfo != nil {
		dmVerityName := fmt.Sprintf(verityDeviceFmt, controller, lun, partition, config.VerityInfo.RootDigest)
		if err := removeDevice(dmVerityName); err != nil {
			// Ignore failures, since the path has been unmounted at this point.
			log.G(ctx).WithError(err).Debugf("failed to remove dm verity target: %s", dmVerityName)
		}
	}

	if config.Encrypted {
		dmCryptName := fmt.Sprintf(cryptDeviceFmt, controller, lun, partition)
		if err := cleanupCryptDevice(ctx, dmCryptName); err != nil {
			return fmt.Errorf("failed to cleanup dm-crypt target %s: %w", dmCryptName, err)
		}
	}

	return nil
}

// GetDevicePath finds the `/dev/sd*` path to the SCSI device on `controller`
// index `lun` with partition index `partition` and also ensures that the device
// is available under that path or context is canceled.
func GetDevicePath(ctx context.Context, controller, lun uint8, partition uint64) (_ string, err error) {
	ctx, span := otelutil.StartSpan(ctx, "scsi::GetDevicePath", trace.WithAttributes(
		attribute.Int64("controller", int64(controller)),
		attribute.Int64("lun", int64(lun)),
		attribute.Int64("partition", int64(partition))))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	scsiID := fmt.Sprintf("%d:0:0:%d", controller, lun)
	// Devices matching the given SCSI code should each have a subdirectory
	// under /sys/bus/scsi/devices/<scsiID>/block.
	blockPath := filepath.Join(scsiDevicesPath, scsiID, "block")
	var deviceNames []os.DirEntry
	for {
		deviceNames, err = osReadDir(blockPath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if len(deviceNames) == 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			default:
				time.Sleep(time.Millisecond * 10)
				continue
			}
		}
		break
	}

	if len(deviceNames) > 1 {
		return "", errors.Errorf("more than one block device could match SCSI ID \"%s\"", scsiID)
	}
	deviceName := deviceNames[0].Name()

	// devices that have partitions have a subdirectory under
	// /sys/bus/scsi/devices/<scsiID>/block/<deviceName> for each partition.
	// Partitions use 1-based indexing, so if `partition` is 0, then we should
	// return the device name without a partition index.
	if partition != 0 {
		partitionName := fmt.Sprintf("%s%d", deviceName, partition)
		partitionPath := filepath.Join(blockPath, deviceName, partitionName)

		// Wait for the device partition to show up
		for {
			fi, err := osStat(partitionPath)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return "", err
			} else if fi == nil {
				// if the fileinfo is nil that means we didn't find the device, keep
				// trying until the context is done or the device path shows up
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				default:
					time.Sleep(time.Millisecond * 10)
					continue
				}
			}
			break
		}
		deviceName = partitionName
	}

	devicePath := filepath.Join("/dev", deviceName)
	log.G(ctx).WithField("devicePath", devicePath).Debug("found device path")

	// devicePath can take some time before its actually available under
	// `/dev/sd*`. Retry while we wait for it to show up.
	for {
		if _, err := osStat(devicePath); err != nil {
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, unix.ENXIO) {
				select {
				case <-ctx.Done():
					log.G(ctx).Warnf("context timed out while retrying to find device %s: %v", devicePath, err)
					return "", err
				default:
					time.Sleep(10 * time.Millisecond)
					continue
				}
			}
			return "", err
		}
		break
	}

	return devicePath, nil
}

// UnplugDevice finds the SCSI device on `controller` index `lun` and issues a
// guest initiated unplug.
//
// If the device is not attached returns no error.
func UnplugDevice(ctx context.Context, controller, lun uint8) (err error) {
	_, span := otelutil.StartSpan(ctx, "scsi::UnplugDevice", trace.WithAttributes(
		attribute.Int64("controller", int64(controller)),
		attribute.Int64("lun", int64(lun))))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	scsiID := fmt.Sprintf("%d:0:0:%d", controller, lun)
	f, err := os.OpenFile(filepath.Join(scsiDevicesPath, scsiID, "delete"), os.O_WRONLY, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	if _, err := f.Write([]byte("1\n")); err != nil {
		return err
	}
	return nil
}

var ErrUnknownFilesystem = errors.New("could not get device filesystem type")

// getDeviceFsType finds a device's filesystem.
// Right now we only support checking for ext4. In the future, this may
// be expanded to support xfs or other fs types.
func getDeviceFsType(devicePath string) (string, error) {
	if _tar2ext4IsDeviceExt4(devicePath) {
		return "ext4", nil
	}

	return "", ErrUnknownFilesystem
}
