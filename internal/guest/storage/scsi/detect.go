//go:build linux
// +build linux

package scsi

import (
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"github.com/sirupsen/logrus"
)

var (
	knownScsiDevices = make(map[string]bool)
	actualMappings   = make(map[string]string)
)

// parseScsiDeviceName parses the given name into a SCSI device name format to return the
// controller number and LUN number. Returns error if it is not a valid SCSI device name.
// Expected device format: <host controller>:<bus>:<target>:<LUN>
func parseScsiDeviceName(name string) (controller uint8, lun uint8, _ error) {
	tokens := strings.Split(name, ":")
	if len(tokens) != 4 {
		return 0, 0, fmt.Errorf("invalid scsi device name: %s", name)
	}

	for i, tok := range tokens {
		n, err := strconv.ParseUint(tok, 10, 8)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid scsi device name: %s", name)
		}
		switch i {
		case 0:
			controller = uint8(n)
		case 3:
			lun = uint8(n)
		}
	}
	return
}

// detectNewScsiDevice looks for all the SCSI devices attached on the host compares them with the
// previously known devices and returns the path of that device which can be used for mounting it.
func detectNewScsiDevice(ctx context.Context) (uint8, uint8, error) {
	// all scsi devices show up under /sys/bus/scsi/devices
	// The devices are named as 0:0:0:0, 0:0:0:1, 1:0:0:0 etc.
	// The naming format is as follows:
	// <host controller no.>:<bus>:<target>:<LUN>
	// (Section 3.1 from https://tldp.org/HOWTO/html_single/SCSI-2.4-HOWTO/)

	for {
		devices, err := os.ReadDir(scsiDevicesPath)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to read entries from %s: %w", scsiDevicesPath, err)
		}

		for _, dev := range devices {
			name := path.Base(dev.Name())
			newController, newLUN, err := parseScsiDeviceName(name)
			if err != nil {
				continue
			}
			if _, ok := knownScsiDevices[name]; !ok {
				knownScsiDevices[name] = true
				return newController, newLUN, nil
			}
		}

		select {
		case <-ctx.Done():
			return 0, 0, fmt.Errorf("context timed out waiting for SCSI device")
		default:
			time.Sleep(time.Millisecond * 10)
			continue
		}

	}
}

func removeKnownDevice(controller, LUN uint8) {
	name := fmt.Sprintf("%d:0:0:%d", controller, LUN)
	delete(knownScsiDevices, name)
}

// recordActualSCSIMapping keeps a mapping of SCSI devices that are mapped at different
// controller/LUN number than expected.
func recordActualSCSIMapping(origController, origLUN, actualController, actualLUN uint8) error {
	origPath := fmt.Sprintf("%d:0:0:%d", origController, origLUN)
	actualPath := fmt.Sprintf("%d:0:0:%d", actualController, actualLUN)
	if origPath != actualPath {
		logrus.Debugf("SCSI disk expected at %s, actually attached to: %s", origPath, actualPath)
	}
	if _, ok := actualMappings[origPath]; !ok {
		actualMappings[origPath] = actualPath
	} else {
		return fmt.Errorf("double mapping of scsi device %s", origPath)
	}
	return nil
}

func getActualMapping(origController, origLUN uint8) (actualController, actualLUN uint8) {
	origPath := fmt.Sprintf("%d:0:0:%d", origController, origLUN)
	if actualMapping, ok := actualMappings[origPath]; ok {
		// should never return error
		actualController, actualLUN, _ = parseScsiDeviceName(actualMapping)
	} else {
		return origController, origLUN
	}
	return
}

func removeActualMapping(origController, origLUN uint8) (actualController, actualLUN uint8) {
	origPath := fmt.Sprintf("%d:0:0:%d", origController, origLUN)
	actualController, actualLUN = getActualMapping(origController, origLUN)
	delete(actualMappings, origPath)
	return
}

// Mount is the wrapper on top of the actual Mount call to detect the correct controller and LUN numbers.
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
	actualController, actualLUN, err := detectNewScsiDevice(ctx)
	if err != nil {
		return err
	}
	err = recordActualSCSIMapping(controller, lun, actualController, actualLUN)
	if err != nil {
		return err
	}
	return mount(ctx, actualController, actualLUN, target, readonly, encrypted, options, verityInfo, securityPolicy)
}

// Unmount is the wrapper on top of the actual Unmount call to pass the correct controller and LUN numbers.
func Unmount(
	ctx context.Context,
	controller,
	lun uint8,
	target string,
	encrypted bool,
	verityInfo *guestresource.DeviceVerityInfo,
	securityPolicy securitypolicy.SecurityPolicyEnforcer,
) (err error) {
	controller, lun = getActualMapping(controller, lun)
	return unmount(ctx, controller, lun, target, encrypted, verityInfo, securityPolicy)
}

// UnplugDevice is the wrapper on top of the actual UnplugDevice call to pass the correct controller and LUN numbers.
func UnplugDevice(ctx context.Context, controller, lun uint8) (err error) {
	controller, lun = removeActualMapping(controller, lun)
	removeKnownDevice(controller, lun)
	return unplugDevice(ctx, controller, lun)
}
