//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/guest/storage/pci"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/opencontainers/runc/libcontainer/devices"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

const (
	sysfsDevPathFormat = "/sys/dev/%s/%d:%d"

	charType  = "char"
	blockType = "block"

	vpciDeviceIDTypeLegacy = "vpci"
	vpciDeviceIDType       = "vpci-instance-id"
)

// addAssignedDevice goes through the assigned devices that have been enumerated
// on the spec and updates the spec so that the correct device nodes can be mounted
// into the resulting container by the runtime.
func addAssignedDevice(ctx context.Context, spec *oci.Spec) error {
	for _, d := range spec.Windows.Devices {
		switch d.IDType {
		case vpciDeviceIDTypeLegacy, vpciDeviceIDType:
			// validate that the device is ready
			fullPCIPath, err := pci.FindDeviceFullPath(ctx, d.ID)
			if err != nil {
				return errors.Wrapf(err, "failed to find device pci path for device %v", d)
			}
			// find the device nodes that link to the pci path we just got
			devs, err := devicePathsFromPCIPath(ctx, fullPCIPath)
			if err != nil {
				return errors.Wrapf(err, "failed to find dev node for device %v", d)
			}
			for _, dev := range devs {
				addLinuxDeviceToSpec(ctx, dev, spec, true)
			}
		}
	}

	return nil
}

// devicePathsFromPCIPath takes a sysfs bus path to the pci device assigned into the guest
// and attempts to find the dev nodes in the guest that map to it.
func devicePathsFromPCIPath(ctx context.Context, pciPath string) ([]*devices.Device, error) {
	// get the full pci path to make sure that it's the final path
	pciFullPath, err := filepath.EvalSymlinks(pciPath)
	if err != nil {
		return nil, err
	}

	// get all host dev devices
	hostDevices, err := devices.HostDevices()
	if err != nil {
		return nil, err
	}

	// some drivers create multiple dev nodes associated with the PCI device
	out := []*devices.Device{}

	// find corresponding entries in sysfs
	for _, d := range hostDevices {
		major := d.Rule.Major
		minor := d.Rule.Minor

		deviceTypeString := ""
		switch d.Rule.Type {
		case devices.BlockDevice:
			deviceTypeString = blockType
		case devices.CharDevice:
			deviceTypeString = charType
		default:
			return nil, errors.New("unsupported device type")
		}

		syfsDevPath := fmt.Sprintf(sysfsDevPathFormat, deviceTypeString, major, minor)
		sysfsFullPath, err := filepath.EvalSymlinks(syfsDevPath)
		if err != nil {
			// Some drivers will make dev nodes that do not have a matching block or
			// char device -- skip those.
			log.G(ctx).WithError(err).Debugf("failed to find sysfs path for device %s", d.Path)
			continue
		}
		if strings.HasPrefix(sysfsFullPath, pciFullPath) {
			out = append(out, d)
		}
	}

	if len(out) == 0 {
		return nil, errors.New("failed to find the device nodes for sysfs pci path")
	}
	return out, nil
}

func addLinuxDeviceToSpec(ctx context.Context, hostDevice *devices.Device, spec *oci.Spec, addCgroupDevice bool) {
	rd := oci.LinuxDevice{
		Path:  hostDevice.Path,
		Type:  string(hostDevice.Type),
		Major: hostDevice.Major,
		Minor: hostDevice.Minor,
		UID:   &hostDevice.Uid,
		GID:   &hostDevice.Gid,
	}
	if hostDevice.Major == 0 && hostDevice.Minor == 0 {
		// Invalid device, most likely a symbolic link, skip it.
		return
	}
	found := false
	for i, dev := range spec.Linux.Devices {
		if dev.Path == rd.Path {
			found = true
			spec.Linux.Devices[i] = rd
			break
		}
		if dev.Type == rd.Type && dev.Major == rd.Major && dev.Minor == rd.Minor {
			log.G(ctx).Warnf("The same type '%s', major '%d' and minor '%d', should not be used for multiple devices.", dev.Type, dev.Major, dev.Minor)
		}
	}
	if !found {
		spec.Linux.Devices = append(spec.Linux.Devices, rd)
		if addCgroupDevice {
			deviceCgroup := oci.LinuxDeviceCgroup{
				Allow:  true,
				Type:   string(hostDevice.Type),
				Major:  &hostDevice.Major,
				Minor:  &hostDevice.Minor,
				Access: string(hostDevice.Permissions),
			}
			spec.Linux.Resources.Devices = append(spec.Linux.Resources.Devices, deviceCgroup)
		}
	}
}
