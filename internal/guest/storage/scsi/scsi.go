//go:build linux
// +build linux

package scsi

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/guest/storage"
	"github.com/Microsoft/hcsshim/internal/guest/storage/crypt"
	dm "github.com/Microsoft/hcsshim/internal/guest/storage/devicemapper"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

// Test dependencies
var (
	osMkdirAll  = os.MkdirAll
	osRemoveAll = os.RemoveAll
	unixMount   = unix.Mount

	// controllerLunToName is stubbed to make testing `Mount` easier.
	controllerLunToName = ControllerLunToName
	// createVerityTarget is stubbed for unit testing `Mount`
	createVerityTarget = dm.CreateVerityTarget
	// removeDevice is stubbed for unit testing `Mount`
	removeDevice = dm.RemoveDevice
)

const (
	scsiDevicesPath = "/sys/bus/scsi/devices"
	verityDeviceFmt = "verity-scsi-contr%d-lun%d-%s"
)

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
	verityInfo *guestresource.DeviceVerityInfo,
	securityPolicy securitypolicy.SecurityPolicyEnforcer,
) (err error) {
	spnCtx, span := trace.StartSpan(ctx, "scsi::Mount")
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
		// containers only have read-only layers so only enforce for them
		var deviceHash string
		if verityInfo != nil {
			deviceHash = verityInfo.RootDigest
		}

		err = securityPolicy.EnforceDeviceMountPolicy(target, deviceHash)
		if err != nil {
			return errors.Wrapf(err, "won't mount scsi controller %d lun %d onto %s", controller, lun, target)
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
			osRemoveAll(target)
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
		encryptedSource, err := crypt.EncryptDevice(spnCtx, source)
		if err != nil {
			return errors.Wrapf(err, "failed to mount encrypted device: "+source)
		}
		source = encryptedSource
	}

	for {
		if err := unixMount(source, target, "ext4", flags, data); err != nil {
			// The `source` found by controllerLunToName can take some time
			// before its actually available under `/dev/sd*`. Retry while we
			// wait for `source` to show up.
			if err == unix.ENOENT {
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

// Unmount unmounts a SCSI device mounted at `target`.
//
// If `encrypted` is true, it removes all its associated dm-crypto state.
func Unmount(
	ctx context.Context,
	controller,
	lun uint8,
	target string,
	encrypted bool,
	verityInfo *guestresource.DeviceVerityInfo,
	securityPolicy securitypolicy.SecurityPolicyEnforcer,
) (err error) {
	ctx, span := trace.StartSpan(ctx, "scsi::Unmount")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.Int64Attribute("controller", int64(controller)),
		trace.Int64Attribute("lun", int64(lun)),
		trace.StringAttribute("target", target))

	if err = securityPolicy.EnforceDeviceUnmountPolicy(target); err != nil {
		return errors.Wrapf(err, "unmounting scsi controller %d lun %d from  %s denied by policy", controller, lun, target)
	}

	// Unmount unencrypted device
	if err := storage.UnmountPath(ctx, target, true); err != nil {
		return errors.Wrapf(err, "unmount failed: "+target)
	}

	if verityInfo != nil {
		dmVerityName := fmt.Sprintf(verityDeviceFmt, controller, lun, verityInfo.RootDigest)
		if err := removeDevice(dmVerityName); err != nil {
			// Ignore failures, since the path has been unmounted at this point.
			log.G(ctx).WithError(err).Debugf("failed to remove dm verity target: %s", dmVerityName)
		}
	}

	if encrypted {
		if err := crypt.CleanupCryptDevice(target); err != nil {
			return errors.Wrapf(err, "failed to cleanup dm-crypt state: "+target)
		}
	}

	return nil
}

// ControllerLunToName finds the `/dev/sd*` path to the SCSI device on
// `controller` index `lun`.
func ControllerLunToName(ctx context.Context, controller, lun uint8) (_ string, err error) {
	ctx, span := trace.StartSpan(ctx, "scsi::ControllerLunToName")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.Int64Attribute("controller", int64(controller)),
		trace.Int64Attribute("lun", int64(lun)))

	scsiID := fmt.Sprintf("0:0:%d:%d", controller, lun)

	// Devices matching the given SCSI code should each have a subdirectory
	// under /sys/bus/scsi/devices/<scsiID>/block.
	blockPath := filepath.Join(scsiDevicesPath, scsiID, "block")
	var deviceNames []os.FileInfo
	for {
		deviceNames, err = ioutil.ReadDir(blockPath)
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
	log.G(ctx).WithField("devicePath", devicePath).Debug("found device path")
	return devicePath, nil
}

// UnplugDevice finds the SCSI device on `controller` index `lun` and issues a
// guest initiated unplug.
//
// If the device is not attached returns no error.
func UnplugDevice(ctx context.Context, controller, lun uint8) (err error) {
	_, span := trace.StartSpan(ctx, "scsi::UnplugDevice")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.Int64Attribute("controller", int64(controller)),
		trace.Int64Attribute("lun", int64(lun)))

	scsiID := fmt.Sprintf("0:0:%d:%d", controller, lun)
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
