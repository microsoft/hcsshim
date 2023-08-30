//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sys/unix"

	specInternal "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/hooks"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/guest/network"
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
	for i, m := range spec.Mounts {
		if strings.HasPrefix(m.Source, guestpath.SandboxMountPrefix) {
			sandboxSource := specInternal.SandboxMountSource(sbid, m.Source)

			// filepath.Join cleans the resulting path before returning, so it would resolve the relative path if one was given.
			// Hence, we need to ensure that the resolved path is still under the correct directory
			if !strings.HasPrefix(sandboxSource, specInternal.SandboxMountsDir(sbid)) {
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
	for i, m := range spec.Mounts {
		if strings.HasPrefix(m.Source, guestpath.HugePagesMountPrefix) {
			mountsDir := specInternal.HugePagesMountsDir(sbid)
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

func setupNetworking(ctx context.Context, hostname, id string, spec *oci.Spec) (err error) {
	// Write the hosts
	sandboxHostsContent := network.GenerateEtcHostsContent(ctx, hostname)
	sandboxHostsPath := getSandboxHostsPath(id)
	log.G(ctx).WithField("sandboxHostsPath", sandboxHostsPath).Debug("Printing sandboxHosts Path")
	log.G(ctx).Debug("SandboxHostsContent ", sandboxHostsContent)
	if err := os.WriteFile(sandboxHostsPath, []byte(sandboxHostsContent), 0644); err != nil {
	 	return errors.Wrapf(err, "failed to write sandbox hosts to %q", sandboxHostsPath)
	}

	// Write resolv.conf
	ns, err := getNetworkNamespace(getNetworkNamespaceID(spec))
	log.G(ctx).Debug("Network namespace", ns)
	if err != nil {
		log.G(ctx).Debug("Error getting network namespace")
		return err
	}
	var searches, servers []string
	for _, n := range ns.Adapters() {
		if len(n.DNSSuffix) > 0 {
			searches = network.MergeValues(searches, strings.Split(n.DNSSuffix, ","))
		}
		if len(n.DNSServerList) > 0 {
			servers = network.MergeValues(servers, strings.Split(n.DNSServerList, ","))
		}
	}
	resolvContent, err := network.GenerateResolvConfContent(ctx, searches, servers, nil)
	if err != nil {
		return errors.Wrap(err, "failed to generate sandbox resolv.conf content")
	}
	sandboxResolvPath := getSandboxResolvPath(id)
	log.G(ctx).Debug("sandboxResolvPath", sandboxResolvPath)
	log.G(ctx).Debug("sandboxResolvContent", resolvContent)
	if err := os.WriteFile(sandboxResolvPath, []byte(resolvContent), 0644); err != nil {
		return errors.Wrap(err, "failed to write sandbox resolv.conf")
	}

	return nil
}

func setupWorkloadContainerSpec(ctx context.Context, sbid, id string, spec *oci.Spec) (err error) {
	log.G(ctx).Debug("Inside setupWorkloadContainerSpec")
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

	hostname, err := os.Hostname()
	if hostname == "" {
		return errors.Wrap(err, "failed to get hostname")
	}
	log.G(ctx).WithField("hostname", hostname).Debug("Printing hostname");

	// Set up networking
	log.G(ctx).Debug("Inside setupWorkloadContainerSpec -> Setting up networking");
	err = setupNetworking(ctx, hostname, sbid, spec);
	log.G(ctx).Debug("Inside setupWorkloadContainerSpec -> Completed setting up networking");

	log.G(ctx).Debug("Inside setupWorkloadContainerSpec -> UpdateSandboxMounts")
	// update any sandbox mounts with the sandboxMounts directory path and create files
	if err = updateSandboxMounts(sbid, spec); err != nil {
		return errors.Wrapf(err, "failed to update sandbox mounts for container %v in sandbox %v", id, sbid)
	}

	if err = updateHugePageMounts(sbid, spec); err != nil {
		return errors.Wrapf(err, "failed to update hugepages mounts for container %v in sandbox %v", id, sbid)
	}

	log.G(ctx).Debug("Inside setupWorkloadContainerSpec -> GenerateWorkloadContainerNetworkMounts")
	// Add default mounts for container networking (e.g. /etc/hostname, /etc/hosts),
	// if spec didn't override them explicitly.
	networkingMounts := specInternal.GenerateWorkloadContainerNetworkMounts(sbid, spec)
	spec.Mounts = append(spec.Mounts, networkingMounts...)

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
		// we only support Nvidia gpus right now
		if specHasGPUDevice(spec) {
			// run ldconfig as a `CreateRuntime` hook so that it's executed in the runtime namespace. This is
			// necessary so that when we execute the nvidia hook to assign the device, the runtime can
			// find the nvidia library files.
			ldconfigHook := hooks.NewOCIHook("/sbin/ldconfig", []string{getNvidiaDriversUsrLibPath()}, os.Environ())
			if err := hooks.AddOCIHook(spec, hooks.CreateRuntime, ldconfigHook); err != nil {
				return err
			}
			if err := addNvidiaDeviceHook(ctx, spec); err != nil {
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

	log.G(ctx).Debug("Inside setupWorkloadContainerSpec -> Completed")
	return nil
}
