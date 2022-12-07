//go:build linux
// +build linux

package scsi

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/guest/storage"
	"github.com/Microsoft/hcsshim/internal/guest/storage/crypt"
	dm "github.com/Microsoft/hcsshim/internal/guest/storage/devicemapper"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// Test dependencies.
var (
	osMkdirAll  = os.MkdirAll
	osRemoveAll = os.RemoveAll
	unixMount   = unix.Mount

	// controllerLunToName is stubbed to make testing `Mount` easier.
	controllerLunToName = ControllerLunToName
	// createVerityTarget is stubbed for unit testing `Mount`.
	createVerityTarget = dm.CreateVerityTarget
	// removeDevice is stubbed for unit testing `Mount`.
	removeDevice = dm.RemoveDevice
	// encryptDevice is stubbed for unit testing `mount`
	encryptDevice = crypt.EncryptDevice
	// cleanupCryptDevice is stubbed for unit testing `mount`
	cleanupCryptDevice = crypt.CleanupCryptDevice
	// storageUnmountPath is stubbed for unit testing `unmount`
	storageUnmountPath = storage.UnmountPath
)

const (
	scsiDevicesPath  = "/sys/bus/scsi/devices"
	vmbusDevicesPath = "/sys/bus/vmbus/devices"
	verityDeviceFmt  = "dm-verity-scsi-contr%d-lun%d-%s"
	cryptDeviceFmt   = "dm-crypt-scsi-contr%d-lun%d"
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

// Mount creates a mount from the SCSI device on `controller` index `lun` to
// `target`
//
// `target` will be created. On mount failure the created `target` will be
// automatically cleaned up.
//
// If `encrypted` is set to true, the SCSI device will be encrypted using
// dm-crypt.
func Mount(
	ctx context.Context,
	controller,
	lun uint8,
	target string,
	readonly bool,
	encrypted bool,
	options []string,
	verityInfo *guestresource.DeviceVerityInfo) (err error) {
	spnCtx, span := oc.StartSpan(ctx, "scsi::Mount")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.Int64Attribute("controller", int64(controller)),
		trace.Int64Attribute("lun", int64(lun)))

	source, err := controllerLunToName(spnCtx, controller, lun)
	if err != nil {
		return err
	}

	if readonly {
		var deviceHash string
		if verityInfo != nil {
			deviceHash = verityInfo.RootDigest
		}

		if verityInfo != nil {
			dmVerityName := fmt.Sprintf(verityDeviceFmt, controller, lun, deviceHash)
			if source, err = createVerityTarget(spnCtx, source, dmVerityName, verityInfo); err != nil {
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

	if encrypted {
		cryptDeviceName := fmt.Sprintf(cryptDeviceFmt, controller, lun)
		encryptedSource, err := encryptDevice(spnCtx, source, cryptDeviceName)
		if err != nil {
			return fmt.Errorf("failed to mount encrypted device %s: %w", source, err)
		}
		source = encryptedSource
	}

	// FIXME: since ControllerLunToName waits for the device to appear under /dev/sd*
	// do we still need this retry loop?
	for {
		if err := unixMount(source, target, "ext4", flags, data); err != nil {
			// The `source` found by controllerLunToName can take some time
			// before its actually available under `/dev/sd*`. Retry while we
			// wait for `source` to show up.
			if errors.Is(err, unix.ENOENT) {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					time.Sleep(10 * time.Millisecond)
					continue
				}
			}
			return err
		}
		break
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
	target string,
	encrypted bool,
	verityInfo *guestresource.DeviceVerityInfo,
) (err error) {
	ctx, span := oc.StartSpan(ctx, "scsi::Unmount")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.Int64Attribute("controller", int64(controller)),
		trace.Int64Attribute("lun", int64(lun)),
		trace.StringAttribute("target", target))

	// unmount target
	if err := storageUnmountPath(ctx, target, true); err != nil {
		return errors.Wrapf(err, "unmount failed: %s", target)
	}

	if verityInfo != nil {
		dmVerityName := fmt.Sprintf(verityDeviceFmt, controller, lun, verityInfo.RootDigest)
		if err := removeDevice(dmVerityName); err != nil {
			// Ignore failures, since the path has been unmounted at this point.
			log.G(ctx).WithError(err).Debugf("failed to remove dm verity target: %s", dmVerityName)
		}
	}

	if encrypted {
		dmCryptName := fmt.Sprintf(cryptDeviceFmt, controller, lun)
		if err := cleanupCryptDevice(dmCryptName); err != nil {
			return fmt.Errorf("failed to cleanup dm-crypt target %s: %w", dmCryptName, err)
		}
	}

	return nil
}

// ControllerLunToName finds the `/dev/sd*` path to the SCSI device on
// `controller` index `lun`.
func ControllerLunToName(ctx context.Context, controller, lun uint8) (_ string, err error) {
	ctx, span := oc.StartSpan(ctx, "scsi::ControllerLunToName")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.Int64Attribute("controller", int64(controller)),
		trace.Int64Attribute("lun", int64(lun)))

	scsiID := fmt.Sprintf("%d:0:0:%d", controller, lun)
	// Devices matching the given SCSI code should each have a subdirectory
	// under /sys/bus/scsi/devices/<scsiID>/block.
	blockPath := filepath.Join(scsiDevicesPath, scsiID, "block")
	var deviceNames []os.DirEntry
	for {
		deviceNames, err = os.ReadDir(blockPath)
		if err != nil && !os.IsNotExist(err) {
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

	devicePath := filepath.Join("/dev", deviceNames[0].Name())
	// wait for device to appear at /dev/sd*
	for {
		_, err = os.Stat(devicePath)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
			time.Sleep(time.Millisecond * 10)
		}
	}
	log.G(ctx).WithField("devicePath", devicePath).Debug("found device path")
	return devicePath, nil
}

// UnplugDevice finds the SCSI device on `controller` index `lun` and issues a
// guest initiated unplug.
//
// If the device is not attached returns no error.
func UnplugDevice(ctx context.Context, controller, lun uint8) (err error) {
	_, span := oc.StartSpan(ctx, "scsi::UnplugDevice")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.Int64Attribute("controller", int64(controller)),
		trace.Int64Attribute("lun", int64(lun)))

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
