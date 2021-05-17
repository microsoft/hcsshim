// +build linux

package scsi

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/Microsoft/opengcs/internal/log"
	"github.com/Microsoft/opengcs/internal/oc"
	"github.com/Microsoft/opengcs/internal/storage"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sys/unix"
)

// Test dependencies
var (
	osMkdirAll  = os.MkdirAll
	osRemoveAll = os.RemoveAll
	unixMount   = unix.Mount

	// controllerLunToName is stubbed to make testing `Mount` easier.
	controllerLunToName = ControllerLunToName
)

// Mount creates a mount from the SCSI device on `controller` index `lun` to
// `target`
//
// `target` will be created. On mount failure the created `target` will be
// automatically cleaned up.
func Mount(ctx context.Context, controller, lun uint8, target string, readonly bool, options []string) (err error) {
	ctx, span := trace.StartSpan(ctx, "scsi::Mount")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.Int64Attribute("controller", int64(controller)),
		trace.Int64Attribute("lun", int64(lun)))

	if err := osMkdirAll(target, 0700); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			osRemoveAll(target)
		}
	}()
	source, err := controllerLunToName(ctx, controller, lun)
	if err != nil {
		return err
	}

	// we only care about readonly mount option when mounting the device
	var flags uintptr
	data := ""
	if readonly {
		flags |= unix.MS_RDONLY
		data = "noload"
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
	blockPath := filepath.Join("/sys/bus/scsi/devices", scsiID, "block")
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

	if len(deviceNames) == 0 {
		return "", errors.Errorf("no matching device names found for SCSI ID \"%s\"", scsiID)
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
	f, err := os.OpenFile(filepath.Join("/sys/bus/scsi/devices", scsiID, "delete"), os.O_WRONLY, 0644)
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
