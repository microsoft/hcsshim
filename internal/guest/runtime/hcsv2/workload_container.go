//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opencontainers/runc/libcontainer/devices"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sys/unix"

	specGuest "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/pkg/annotations"
)

// os.MkdirAll combines the given permissions with the running process's
// umask. By default this causes 0777 to become 0755.
// Temporarily set the umask of this process to 0 so that we can actually
// make all dirs with os.ModePerm permissions.
func mkdirAllModePerm(target string) error {
	savedUmask := unix.Umask(0)
	defer unix.Umask(savedUmask)
	return os.MkdirAll(target, os.ModePerm)
}

func updateSandboxMounts(sbid string, spec *oci.Spec) error {
	// Check if this is a virtual pod
	virtualSandboxID := spec.Annotations[annotations.VirtualPodID]

	for i, m := range spec.Mounts {
		if !strings.HasPrefix(m.Source, guestpath.SandboxMountPrefix) &&
			!strings.HasPrefix(m.Source, guestpath.SandboxTmpfsMountPrefix) {
			continue
		}

		var sandboxSource string
		// if using `sandbox-tmp://` prefix, we mount a tmpfs in sandboxTmpfsMountsDir
		if strings.HasPrefix(m.Source, guestpath.SandboxTmpfsMountPrefix) {
			// Use virtual pod aware mount source
			sandboxSource = specGuest.VirtualPodAwareSandboxTmpfsMountSource(sbid, virtualSandboxID, m.Source)
			expectedMountsDir := specGuest.VirtualPodAwareSandboxTmpfsMountsDir(sbid, virtualSandboxID)

			// filepath.Join cleans the resulting path before returning, so it would resolve the relative path if one was given.
			// Hence, we need to ensure that the resolved path is still under the correct directory
			if !strings.HasPrefix(sandboxSource, expectedMountsDir) {
				return errors.Errorf("mount path %v for mount %v is not within sandbox's tmpfs mounts dir", sandboxSource, m.Source)
			}
		} else {
			// Use virtual pod aware mount source
			sandboxSource = specGuest.VirtualPodAwareSandboxMountSource(sbid, virtualSandboxID, m.Source)
			expectedMountsDir := specGuest.VirtualPodAwareSandboxMountsDir(sbid, virtualSandboxID)

			// filepath.Join cleans the resulting path before returning, so it would resolve the relative path if one was given.
			// Hence, we need to ensure that the resolved path is still under the correct directory
			if !strings.HasPrefix(sandboxSource, expectedMountsDir) {
				return errors.Errorf("mount path %v for mount %v is not within sandbox's mounts dir", sandboxSource, m.Source)
			}
		}

		spec.Mounts[i].Source = sandboxSource

		_, err := os.Stat(sandboxSource)
		if os.IsNotExist(err) {
			if err := mkdirAllModePerm(sandboxSource); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateHugePageMounts(sbid string, spec *oci.Spec) error {
	// Check if this is a virtual pod
	virtualSandboxID := spec.Annotations[annotations.VirtualPodID]

	for i, m := range spec.Mounts {
		if !strings.HasPrefix(m.Source, guestpath.HugePagesMountPrefix) {
			continue
		}

		// Use virtual pod aware hugepages directory
		mountsDir := specGuest.VirtualPodAwareHugePagesMountsDir(sbid, virtualSandboxID)
		subPath := strings.TrimPrefix(m.Source, guestpath.HugePagesMountPrefix)
		pageSize := strings.Split(subPath, string(os.PathSeparator))[0]
		hugePageMountSource := filepath.Join(mountsDir, subPath)

		// filepath.Join cleans the resulting path before returning so it would resolve the relative path if one was given.
		// Hence, we need to ensure that the resolved path is still under the correct directory
		if !strings.HasPrefix(hugePageMountSource, mountsDir) {
			return errors.Errorf("mount path %v for mount %v is not within hugepages's mounts dir", hugePageMountSource, m.Source)
		}

		spec.Mounts[i].Source = hugePageMountSource

		_, err := os.Stat(hugePageMountSource)
		if os.IsNotExist(err) {
			if err := mkdirAllModePerm(hugePageMountSource); err != nil {
				return err
			}
			if err := unix.Mount("none", hugePageMountSource, "hugetlbfs", 0, "pagesize="+pageSize); err != nil {
				return errors.Errorf("mount operation failed for %v failed with error %v", hugePageMountSource, err)
			}
		}
	}
	return nil
}

func updateBlockDeviceMounts(spec *oci.Spec) error {
	for i, m := range spec.Mounts {
		if !strings.HasPrefix(m.Destination, guestpath.BlockDevMountPrefix) {
			continue
		}
		permissions := "rwm"
		for _, o := range m.Options {
			if o == "ro" {
				permissions = "r"
				break
			}
		}

		// For block device mounts, the source will be a symlink. Resolve it first
		// before passing to `DeviceFromPath`, which expects a real device path.
		rPath, err := os.Readlink(m.Source)
		if err != nil {
			return fmt.Errorf("failed to readlink %s: %w", m.Source, err)
		}

		sourceDevice, err := devices.DeviceFromPath(rPath, permissions)
		if err != nil {
			return fmt.Errorf("failed to get device from path: %w", err)
		}

		deviceCgroup := oci.LinuxDeviceCgroup{
			Allow:  true,
			Type:   string(sourceDevice.Type),
			Major:  &sourceDevice.Major,
			Minor:  &sourceDevice.Minor,
			Access: string(sourceDevice.Permissions),
		}

		spec.Linux.Resources.Devices = append(spec.Linux.Resources.Devices, deviceCgroup)
		spec.Mounts[i].Destination = strings.TrimPrefix(m.Destination, guestpath.BlockDevMountPrefix)
	}
	return nil
}

func updateUVMMounts(spec *oci.Spec) error {
	for i, m := range spec.Mounts {
		if !strings.HasPrefix(m.Source, guestpath.UVMMountPrefix) {
			continue
		}
		uvmPath := strings.TrimPrefix(m.Source, guestpath.UVMMountPrefix)

		spec.Mounts[i].Source = uvmPath

		if _, err := os.Stat(uvmPath); err != nil {
			return errors.Wrap(err, "could not open uVM mount target")
		}
	}
	return nil
}

func specHasGPUDevice(spec *oci.Spec) bool {
	for _, d := range spec.Windows.Devices {
		if d.IDType == "gpu" {
			return true
		}
	}
	return false
}

func setupWorkloadContainerSpec(ctx context.Context, sbid, id string, spec *oci.Spec, ociBundlePath string) (err error) {
	ctx, span := oc.StartSpan(ctx, "hcsv2::setupWorkloadContainerSpec")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("sandboxID", sbid),
		trace.StringAttribute("cid", id))

	// Verify no hostname
	if spec.Hostname != "" {
		return errors.Errorf("workload container must not change hostname: %s", spec.Hostname)
	}

	// update any sandbox mounts with the sandboxMounts directory path and create files
	if err = updateSandboxMounts(sbid, spec); err != nil {
		return errors.Wrapf(err, "failed to update sandbox mounts for container %v in sandbox %v", id, sbid)
	}

	if err = updateHugePageMounts(sbid, spec); err != nil {
		return errors.Wrapf(err, "failed to update hugepages mounts for container %v in sandbox %v", id, sbid)
	}

	if err = updateBlockDeviceMounts(spec); err != nil {
		return fmt.Errorf("failed to update block device mounts for container %v in sandbox %v: %w", id, sbid, err)
	}

	if err = updateUVMMounts(spec); err != nil {
		return errors.Wrapf(err, "failed to update uVM mounts for container %v in sandbox %v", id, sbid)
	}

	// Add default mounts for container networking (e.g. /etc/hostname, /etc/hosts),
	// if spec didn't override them explicitly.
	networkingMounts := specGuest.GenerateWorkloadContainerNetworkMounts(sbid, spec)
	spec.Mounts = append(spec.Mounts, networkingMounts...)

	// TODO: JTERRY75 /dev/shm is not properly setup for LCOW I believe. CRI
	// also has a concept of a sandbox/shm file when the IPC NamespaceMode !=
	// NODE.

	if err := specGuest.ApplyAnnotationsToSpec(ctx, spec); err != nil {
		return err
	}

	if rlimCore := spec.Annotations[annotations.RLimitCore]; rlimCore != "" {
		if err := specGuest.SetCoreRLimit(spec, rlimCore); err != nil {
			return err
		}
	}

	// User.Username is generally only used on Windows, but as there's no (easy/fast at least) way to grab
	// a uid:gid pairing for a username string on the host, we need to defer this work until we're here in the
	// guest. The username field is used as a temporary holding place until we can perform this work here when
	// we actually have the rootfs to inspect.
	if spec.Process.User.Username != "" {
		if err := specGuest.SetUserStr(spec, spec.Process.User.Username); err != nil {
			return err
		}
	}

	// Check if this is a virtual pod container
	virtualPodID := spec.Annotations[annotations.VirtualPodID]

	// Set cgroup path - check if this is a virtual pod container
	if virtualPodID != "" {
		// Virtual pod containers go under /containers/virtual-pods/virtualPodID/containerID
		spec.Linux.CgroupsPath = "/containers/virtual-pods/" + virtualPodID + "/" + id
	} else {
		// Regular containers go under /containers
		spec.Linux.CgroupsPath = "/containers/" + id
	}

	if spec.Windows != nil {
		// we only support Nvidia gpus right now
		if specHasGPUDevice(spec) {
			if err := addNvidiaDeviceHook(ctx, spec, ociBundlePath); err != nil {
				return err
			}

			// The NVIDIA device hook `nvidia-container-cli` adds `rw` permissions for the
			// GPU and ctl nodes (`c 195:*`) to the  devices allow list, but CUDA apparently also
			// needs `rwm` permission for other device nodes (e.g., `c 235`)
			//
			// Grant `rwm` to all character devices (`c *:* rwm`) to avoid hard coding exact node
			// numbers, which are unknown before the driver runs (GPU devices are presented as I2C
			// devices initially) or could change with driver implementation.
			//
			// Note: runc already grants mknod, `c *:* m`, so this really adds `rw` permissions for
			// all character devices:
			// https://github.com/opencontainers/runc/blob/6bae6cad4759a5b3537d550f43ea37d51c6b518a/libcontainer/specconv/spec_linux.go#L205-L222
			spec.Linux.Resources.Devices = append(spec.Linux.Resources.Devices,
				oci.LinuxDeviceCgroup{
					Allow:  true,
					Type:   "c",
					Access: "rwm",
				},
			)
		}
		// add other assigned devices to the spec
		if err := specGuest.AddAssignedDevice(ctx, spec); err != nil {
			return errors.Wrap(err, "failed to add assigned device(s) to the container spec")
		}
	}

	// Clear the windows section as we dont want to forward to runc
	spec.Windows = nil

	return nil
}
