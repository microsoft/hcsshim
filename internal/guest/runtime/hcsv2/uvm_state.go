//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/sirupsen/logrus"
)

type deviceType int

const (
	DeviceTypeRW deviceType = iota
	DeviceTypeRO
	DeviceTypeOverlay
)

func (d deviceType) String() string {
	switch d {
	case DeviceTypeRW:
		return "RW"
	case DeviceTypeRO:
		return "RO"
	case DeviceTypeOverlay:
		return "Overlay"
	default:
		return fmt.Sprintf("Unknown(%d)", d)
	}
}

type device struct {
	// fields common to all
	mountPath  string
	ty         deviceType
	usage      int
	sourcePath string

	// rw devices
	encrypted bool

	// overlay devices
	referencedDevices []*device
}

// hostMounts tracks the state of fs/overlay mounts and their usage
// relationship. Users of this struct must call hm.Lock() before calling any
// other methods and call hm.Unlock() when done.
//
// Since mount/unmount operations can fail, the expected way to use this struct
// is to first lock it, call the method to add/remove the device, then, with the
// lock still held, perform the actual operation.  If the operation fails, the
// caller must undo the operation by calling the appropriate remove/add method
// or the returned undo function, before unlocking.
type hostMounts struct {
	stateMutex       sync.Mutex
	stateMutexLocked bool

	// Map from mountPath to device struct
	devices map[string]*device
}

func newHostMounts() *hostMounts {
	return &hostMounts{
		devices: make(map[string]*device),
	}
}

func (hm *hostMounts) expectLocked() {
	if !hm.stateMutexLocked {
		gcs.UnrecoverableError(errors.New("hostMounts: expected stateMutex to be locked, but it was not"))
	}
}

// Locks the state mutex. This is not re-entrant, calling it twice in the same
// thread will deadlock/panic.
func (hm *hostMounts) Lock() {
	hm.stateMutex.Lock()
	// Since we just acquired the lock, either it was not locked before, or
	// somebody just unlocked it. Either case, hm.stateMutexLocked should be
	// false.
	if hm.stateMutexLocked {
		gcs.UnrecoverableError(errors.New("hostMounts: stateMutexLocked already true when locking stateMutex"))
	}
	hm.stateMutexLocked = true
}

// Unlocks the state mutex
func (hm *hostMounts) Unlock() {
	hm.expectLocked()
	hm.stateMutexLocked = false
	hm.stateMutex.Unlock()
}

func (hm *hostMounts) findDeviceAtPath(mountPath string) *device {
	hm.expectLocked()

	if dev, ok := hm.devices[mountPath]; ok {
		return dev
	}
	return nil
}

func (hm *hostMounts) addDeviceToMapChecked(dev *device) error {
	hm.expectLocked()

	if _, ok := hm.devices[dev.mountPath]; ok {
		return fmt.Errorf("device at mount path %q already exists", dev.mountPath)
	}
	hm.devices[dev.mountPath] = dev
	return nil
}

func (hm *hostMounts) findDeviceContainingPath(path string) *device {
	hm.expectLocked()

	// TODO: can we refactor this function by walking each component of the path
	// from leaf to root, each time checking if the current component is a mount
	// point?  (i.e. why do we have to use filepath.Rel?)

	var foundDev *device
	cleanPath := filepath.Clean(path)
	for devPath, dev := range hm.devices {
		relPath, err := filepath.Rel(devPath, cleanPath)
		// skip further checks if an error is returned or the relative path
		// contains "..", meaning that the `path` isn't directly nested under
		// `rwPath`.
		if err != nil || strings.HasPrefix(relPath, "..") {
			continue
		}
		if foundDev == nil {
			foundDev = dev
		} else if len(dev.mountPath) > len(foundDev.mountPath) {
			// The current device is mounted on top of a previously found device.
			foundDev = dev
		}
	}
	return foundDev
}

func (hm *hostMounts) usePath(path string) (*device, error) {
	hm.expectLocked()

	// Find the device at the given path and increment its usage count.
	dev := hm.findDeviceContainingPath(path)
	if dev == nil {
		return nil, nil
	}
	dev.usage++
	return dev, nil
}

func (hm *hostMounts) releaseDeviceUsage(dev *device) {
	hm.expectLocked()

	if dev.usage <= 0 {
		log.G(context.Background()).WithFields(logrus.Fields{
			"device":       dev.mountPath,
			"deviceSource": dev.sourcePath,
			"deviceType":   dev.ty,
			"usage":        dev.usage,
		}).Error("hostMounts::releaseDeviceUsage: unexpected zero usage count")
		return
	}
	dev.usage--
}

// User should carefully handle side-effects of adding a device if the device
// fails to be added.
func (hm *hostMounts) doAddDevice(mountPath string, ty deviceType, sourcePath string) (*device, error) {
	hm.expectLocked()

	dev := &device{
		mountPath:  filepath.Clean(mountPath),
		ty:         ty,
		usage:      0,
		sourcePath: sourcePath,
	}

	if err := hm.addDeviceToMapChecked(dev); err != nil {
		return nil, err
	}
	return dev, nil
}

// Once checks is called, unless it returns an error, this function will always
// succeed
func (hm *hostMounts) doRemoveDevice(mountPath string, ty deviceType, sourcePath string, checks func(*device) error) error {
	hm.expectLocked()

	unmountTarget := filepath.Clean(mountPath)
	device := hm.findDeviceAtPath(unmountTarget)
	if device == nil {
		// already removed or didn't exist
		return nil
	}
	if device.sourcePath != sourcePath {
		return fmt.Errorf("wrong sourcePath %s, expected %s", sourcePath, device.sourcePath)
	}
	if device.ty != ty {
		return fmt.Errorf("wrong device type %s, expected %s", ty, device.ty)
	}
	if device.usage > 0 {
		log.G(context.Background()).WithFields(logrus.Fields{
			"device":       device.mountPath,
			"deviceSource": device.sourcePath,
			"deviceType":   device.ty,
			"usage":        device.usage,
		}).Error("hostMounts::doRemoveDevice: device still in use, refusing unmount")
		return fmt.Errorf("device at %q is still in use, can't unmount", unmountTarget)
	}
	if checks != nil {
		if err := checks(device); err != nil {
			return err
		}
	}

	delete(hm.devices, unmountTarget)
	return nil
}

func (hm *hostMounts) AddRODevice(mountPath string, sourcePath string) error {
	hm.expectLocked()

	_, err := hm.doAddDevice(mountPath, DeviceTypeRO, sourcePath)
	return err
}

// AddRWDevice adds read-write device metadata for device mounted at `mountPath`.
// Returns an error if there's an existing device mounted at `mountPath` location.
func (hm *hostMounts) AddRWDevice(mountPath string, sourcePath string, encrypted bool) error {
	hm.expectLocked()

	dev, err := hm.doAddDevice(mountPath, DeviceTypeRW, sourcePath)
	if err != nil {
		return err
	}
	dev.encrypted = encrypted
	return nil
}

func (hm *hostMounts) AddOverlay(mountPath string, layers []string, scratchDir string) (err error) {
	hm.expectLocked()

	dev, err := hm.doAddDevice(mountPath, DeviceTypeOverlay, mountPath)
	if err != nil {
		return err
	}
	dev.referencedDevices = make([]*device, 0, len(layers)+1)
	defer func() {
		if err != nil {
			// If we failed to use any of the paths, we need to release the ones
			// that we did use.
			for _, d := range dev.referencedDevices {
				hm.releaseDeviceUsage(d)
			}
			delete(hm.devices, mountPath)
		}
	}()

	for _, layer := range layers {
		refDev, err := hm.usePath(layer)
		if err != nil {
			return err
		}
		if refDev != nil {
			dev.referencedDevices = append(dev.referencedDevices, refDev)
		}
	}
	refDev, err := hm.usePath(scratchDir)
	if err != nil {
		return err
	}
	if refDev != nil {
		dev.referencedDevices = append(dev.referencedDevices, refDev)
	}

	return nil
}

func (hm *hostMounts) RemoveRODevice(mountPath string, sourcePath string) error {
	hm.expectLocked()

	return hm.doRemoveDevice(mountPath, DeviceTypeRO, sourcePath, nil)
}

// RemoveRWDevice removes the read-write device metadata for device mounted at
// `mountPath`.
func (hm *hostMounts) RemoveRWDevice(mountPath string, sourcePath string, encrypted bool) error {
	hm.expectLocked()

	return hm.doRemoveDevice(mountPath, DeviceTypeRW, sourcePath, func(dev *device) error {
		if dev.encrypted != encrypted {
			return fmt.Errorf("encrypted flag wrong, provided %v, expected %v", encrypted, dev.encrypted)
		}
		return nil
	})
}

func (hm *hostMounts) RemoveOverlay(mountPath string) (undo func(), err error) {
	hm.expectLocked()

	var dev *device
	err = hm.doRemoveDevice(mountPath, DeviceTypeOverlay, mountPath, func(_dev *device) error {
		dev = _dev
		for _, refDev := range dev.referencedDevices {
			hm.releaseDeviceUsage(refDev)
		}
		return nil
	})
	if err != nil {
		// If we get an error from doRemoveDevice, we have not released anything
		// yet.
		return nil, err
	}
	undo = func() {
		hm.expectLocked()

		for _, refDev := range dev.referencedDevices {
			refDev.usage++
		}

		if _, ok := hm.devices[mountPath]; ok {
			log.G(context.Background()).WithField("mountPath", mountPath).Error(
				"hostMounts::RemoveOverlay: failed to undo remove: device that was removed exists in map",
			)
			return
		}

		hm.devices[mountPath] = dev
	}
	return undo, nil
}

// IsEncrypted checks if the given path is a sub-path of an encrypted read-write
// device.
func (hm *hostMounts) IsEncrypted(path string) bool {
	hm.expectLocked()

	dev := hm.findDeviceContainingPath(path)
	if dev == nil {
		return false
	}
	return dev.encrypted
}

func (hm *hostMounts) HasOverlayMountedAt(path string) bool {
	hm.expectLocked()

	dev := hm.findDeviceAtPath(filepath.Clean(path))
	if dev == nil {
		return false
	}
	return dev.ty == DeviceTypeOverlay
}
