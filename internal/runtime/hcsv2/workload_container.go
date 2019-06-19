package hcsv2

import (
	"context"
	"path/filepath"
	"time"

	"github.com/opencontainers/runc/libcontainer/devices"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func getWorkloadRootDir(sbid, id string) string {
	return filepath.Join(getSandboxRootDir(sbid), id)
}

func setupWorkloadContainerSpec(ctx context.Context, sbid, id string, spec *oci.Spec) (err error) {
	// TODO: JTERRY75 use ctx for log
	operation := "setupWorkloadContainerSpec"
	start := time.Now()
	defer func() {
		end := time.Now()
		fields := logrus.Fields{
			"cid":       id,
			"startTime": start,
			"endTime":   end,
			"duration":  end.Sub(start),
		}
		if err != nil {
			fields[logrus.ErrorKey] = err
			logrus.WithFields(fields).Error(operation)
		} else {
			logrus.WithFields(fields).Info(operation)
		}
	}()

	// Verify no hostname
	if spec.Hostname != "" {
		return errors.Errorf("workload container must not change hostname: %s", spec.Hostname)
	}

	// Add /etc/hostname if the spec did not override it.
	if !isInMounts("/etc/hostname", spec.Mounts) {
		mt := oci.Mount{
			Destination: "/etc/hostname",
			Type:        "bind",
			Source:      getSandboxHostnamePath(sbid),
			Options:     []string{"bind"},
		}
		if isRootReadonly(spec) {
			mt.Options = append(mt.Options, "ro")
		}
		spec.Mounts = append(spec.Mounts, mt)
	}

	// Add /etc/hosts if the spec did not override it.
	if !isInMounts("/etc/hosts", spec.Mounts) {
		mt := oci.Mount{
			Destination: "/etc/hosts",
			Type:        "bind",
			Source:      getSandboxHostsPath(sbid),
			Options:     []string{"bind"},
		}
		if isRootReadonly(spec) {
			mt.Options = append(mt.Options, "ro")
		}
		spec.Mounts = append(spec.Mounts, mt)
	}

	// Add /etc/resolv.conf if the spec did not override it.
	if !isInMounts("/etc/resolv.conf", spec.Mounts) {
		mt := oci.Mount{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      getSandboxResolvPath(sbid),
			Options:     []string{"bind"},
		}
		if isRootReadonly(spec) {
			mt.Options = append(mt.Options, "ro")
		}
		spec.Mounts = append(spec.Mounts, mt)
	}

	// TODO: JTERRY75 /dev/shm is not properly setup for LCOW I believe. CRI
	// also has a concept of a sandbox/shm file when the IPC NamespaceMode !=
	// NODE.

	// Check if we need to do any capability/device mappings
	if spec.Annotations["io.microsoft.virtualmachine.lcow.privileged"] == "true" {
		logrus.WithFields(logrus.Fields{
			"cid": id,
		}).Debugf("setupWorkloadContainerSpec - 'io.microsoft.virtualmachine.lcow.privileged' set for privileged container")

		// Add all host devices
		hostDevices, err := devices.HostDevices()
		if err != nil {
			return err
		}
		for _, hostDevice := range hostDevices {
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
				continue
			}
			found := false
			for i, dev := range spec.Linux.Devices {
				if dev.Path == rd.Path {
					found = true
					spec.Linux.Devices[i] = rd
					break
				}
				if dev.Type == rd.Type && dev.Major == rd.Major && dev.Minor == rd.Minor {
					logrus.WithFields(logrus.Fields{
						"cid": id,
					}).Warnf("setupWorkloadContainerSpec - The same type '%s', major '%d' and minor '%d', should not be used for multiple devices.", dev.Type, dev.Major, dev.Minor)
				}
			}
			if !found {
				spec.Linux.Devices = append(spec.Linux.Devices, rd)
			}
		}

		// Set the cgroup access
		spec.Linux.Resources.Devices = []oci.LinuxDeviceCgroup{
			{
				Allow:  true,
				Access: "rwm",
			},
		}
	}

	// Clear the windows section as we dont want to forward to runc
	spec.Windows = nil

	return nil
}
