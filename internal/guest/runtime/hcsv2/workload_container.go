// +build linux

package hcsv2

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sys/unix"
)

func getWorkloadRootDir(id string) string {
	return filepath.Join("/run/gcs/c", id)
}

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
	sandboxMountPrefix := "sandbox://"
	for i, m := range spec.Mounts {
		if strings.HasPrefix(m.Source, sandboxMountPrefix) {
			mountsDir := getSandboxMountsDir(sbid)
			subPath := strings.TrimPrefix(m.Source, sandboxMountPrefix)
			sandboxSource := filepath.Join(mountsDir, subPath)

			// filepath.Join cleans the resulting path before returning so it would resolve the relative path if one was given.
			// Hence, we need to ensure that the resolved path is still under the correct directory
			if !strings.HasPrefix(sandboxSource, mountsDir) {
				return errors.Errorf("mount path %v for mount %v is not within sandbox's mounts dir", sandboxSource, m.Source)
			}

			spec.Mounts[i].Source = sandboxSource

			_, err := os.Stat(sandboxSource)
			if os.IsNotExist(err) {
				if err := mkdirAllModePerm(sandboxSource); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func updateHugePageMounts(sbid string, spec *oci.Spec) error {
	mountPrefix := "hugepages://"
	for i, m := range spec.Mounts {
		if strings.HasPrefix(m.Source, mountPrefix) {
			mountsDir := getSandboxHugePageMountsDir(sbid)
			subPath := strings.TrimPrefix(m.Source, mountPrefix)
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

func setupWorkloadContainerSpec(ctx context.Context, sbid, id string, spec *oci.Spec) (err error) {
	ctx, span := trace.StartSpan(ctx, "hcsv2::setupWorkloadContainerSpec")
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

	if err := applyAnnotationsToSpec(ctx, spec); err != nil {
		return err
	}

	if rlimCore := spec.Annotations[annotations.RLimitCore]; rlimCore != "" {
		if err := setCoreRLimit(spec, rlimCore); err != nil {
			return err
		}
	}

	// User.Username is generally only used on Windows, but as there's no (easy/fast at least) way to grab
	// a uid:gid pairing for a username string on the host, we need to defer this work until we're here in the
	// guest. The username field is used as a temporary holding place until we can perform this work here when
	// we actually have the rootfs to inspect.
	if spec.Process.User.Username != "" {
		if err := setUserStr(spec, spec.Process.User.Username); err != nil {
			return err
		}
	}

	// Force the parent cgroup into our /containers root
	spec.Linux.CgroupsPath = "/containers/" + id

	if spec.Windows != nil {
		if specHasGPUDevice(spec) {
			// we only support Nvidia gpus right now
			ldConfigargs := []string{"-l", "/run/nvidia/lib"}
			env := updateEnvWithNvidiaVariables()
			if err := addLDConfigHook(ctx, spec, ldConfigargs, env); err != nil {
				return err
			}
			if err := addNvidiaDevicePreHook(ctx, spec); err != nil {
				return err
			}
		}
		// add other assigned devices to the spec
		if err := addAssignedDevice(ctx, spec); err != nil {
			return errors.Wrap(err, "failed to add assigned device(s) to the container spec")
		}
	}

	// Clear the windows section as we dont want to forward to runc
	spec.Windows = nil

	return nil
}
