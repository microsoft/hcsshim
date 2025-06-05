//go:build linux
// +build linux

package spec

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/moby/sys/user"
	"github.com/opencontainers/runc/libcontainer/devices"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
)

const (
	devShmPath = "/dev/shm"
)

// networkingMountPaths returns an array of mount paths to enable networking
// inside containers.
func networkingMountPaths() []string {
	return []string{
		"/etc/hostname",
		"/etc/hosts",
		"/etc/resolv.conf",
	}
}

// GenerateWorkloadContainerNetworkMounts generates an array of specs.Mount
// required for container networking. Original spec is left untouched and
// it's the responsibility of a caller to update it.
func GenerateWorkloadContainerNetworkMounts(sandboxID string, spec *oci.Spec) []oci.Mount {
	var nMounts []oci.Mount

	for _, mountPath := range networkingMountPaths() {
		// Don't override if the mount is present in the spec
		if MountPresent(mountPath, spec.Mounts) {
			continue
		}
		options := []string{"bind"}
		if spec.Root != nil && spec.Root.Readonly {
			options = append(options, "ro")
		}
		trimmedMountPath := strings.TrimPrefix(mountPath, "/etc/")
		mt := oci.Mount{
			Destination: mountPath,
			Type:        "bind",
			Source:      filepath.Join(SandboxRootDir(sandboxID), trimmedMountPath),
			Options:     options,
		}
		nMounts = append(nMounts, mt)
	}
	return nMounts
}

// MountPresent checks if mountPath is present in the specMounts array.
func MountPresent(mountPath string, specMounts []oci.Mount) bool {
	for _, m := range specMounts {
		if m.Destination == mountPath {
			return true
		}
	}
	return false
}

// SandboxRootDir returns the sandbox container root directory inside UVM/host.
func SandboxRootDir(sandboxID string) string {
	return filepath.Join(guestpath.LCOWRootPrefixInUVM, sandboxID)
}

// VirtualPodRootDir returns the virtual pod root directory inside UVM/host.
// This is used when multiple pods share a UVM via virtualSandboxID.
func VirtualPodRootDir(virtualSandboxID string) string {
	// Ensure virtualSandboxID is a relative path to prevent directory traversal
	sanitizedID := filepath.Clean(virtualSandboxID)
	if filepath.IsAbs(sanitizedID) || strings.Contains(sanitizedID, "..") {
		return ""
	}
	return filepath.Join(guestpath.LCOWRootPrefixInUVM, "virtual-pods", sanitizedID)
}

// VirtualPodAwareSandboxRootDir returns the appropriate root directory based on whether
// the sandbox is part of a virtual pod or traditional single-pod setup.
func VirtualPodAwareSandboxRootDir(sandboxID, virtualSandboxID string) string {
	if virtualSandboxID != "" {
		return VirtualPodRootDir(virtualSandboxID)
	}
	return SandboxRootDir(sandboxID)
}

// SandboxMountsDir returns sandbox mounts directory inside UVM/host.
func SandboxMountsDir(sandboxID string) string {
	return filepath.Join(SandboxRootDir(sandboxID), "sandboxMounts")
}

// VirtualPodMountsDir returns virtual pod mounts directory inside UVM/host.
func VirtualPodMountsDir(virtualSandboxID string) string {
	return filepath.Join(VirtualPodRootDir(virtualSandboxID), "sandboxMounts")
}

// VirtualPodAwareSandboxMountsDir returns the appropriate mounts directory.
func VirtualPodAwareSandboxMountsDir(sandboxID, virtualSandboxID string) string {
	if virtualSandboxID != "" {
		return VirtualPodMountsDir(virtualSandboxID)
	}
	return SandboxMountsDir(sandboxID)
}

// SandboxTmpfsMountsDir returns sandbox tmpfs mounts directory inside UVM.
func SandboxTmpfsMountsDir(sandboxID string) string {
	return filepath.Join(SandboxRootDir(sandboxID), "sandboxTmpfsMounts")
}

// VirtualPodTmpfsMountsDir returns virtual pod tmpfs mounts directory inside UVM/host.
func VirtualPodTmpfsMountsDir(virtualSandboxID string) string {
	return filepath.Join(VirtualPodRootDir(virtualSandboxID), "sandboxTmpfsMounts")
}

// VirtualPodAwareSandboxTmpfsMountsDir returns the appropriate tmpfs mounts directory.
func VirtualPodAwareSandboxTmpfsMountsDir(sandboxID, virtualSandboxID string) string {
	if virtualSandboxID != "" {
		return VirtualPodTmpfsMountsDir(virtualSandboxID)
	}
	return SandboxTmpfsMountsDir(sandboxID)
}

// HugePagesMountsDir returns hugepages mounts directory inside UVM.
func HugePagesMountsDir(sandboxID string) string {
	return filepath.Join(SandboxRootDir(sandboxID), "hugepages")
}

// VirtualPodHugePagesMountsDir returns virtual pod hugepages mounts directory.
func VirtualPodHugePagesMountsDir(virtualSandboxID string) string {
	return filepath.Join(VirtualPodRootDir(virtualSandboxID), "hugepages")
}

// VirtualPodAwareHugePagesMountsDir returns the appropriate hugepages directory.
func VirtualPodAwareHugePagesMountsDir(sandboxID, virtualSandboxID string) string {
	if virtualSandboxID != "" {
		return VirtualPodHugePagesMountsDir(virtualSandboxID)
	}
	return HugePagesMountsDir(sandboxID)
}

// SandboxMountSource returns sandbox mount path inside UVM.
func SandboxMountSource(sandboxID, path string) string {
	mountsDir := SandboxMountsDir(sandboxID)
	subPath := strings.TrimPrefix(path, guestpath.SandboxMountPrefix)
	return filepath.Join(mountsDir, subPath)
}

// VirtualPodAwareSandboxMountSource returns mount source path for virtual pod aware containers.
func VirtualPodAwareSandboxMountSource(sandboxID, virtualSandboxID, path string) string {
	if virtualSandboxID != "" {
		mountsDir := VirtualPodMountsDir(virtualSandboxID)
		subPath := strings.TrimPrefix(path, guestpath.SandboxMountPrefix)
		return filepath.Join(mountsDir, subPath)
	}
	return SandboxMountSource(sandboxID, path)
}

// SandboxTmpfsMountSource returns sandbox tmpfs mount path inside UVM.
func SandboxTmpfsMountSource(sandboxID, path string) string {
	tmpfsMountDir := SandboxTmpfsMountsDir(sandboxID)
	subPath := strings.TrimPrefix(path, guestpath.SandboxTmpfsMountPrefix)
	return filepath.Join(tmpfsMountDir, subPath)
}

// VirtualPodAwareSandboxTmpfsMountSource returns tmpfs mount source path for virtual pod aware containers.
func VirtualPodAwareSandboxTmpfsMountSource(sandboxID, virtualSandboxID, path string) string {
	if virtualSandboxID != "" {
		mountsDir := VirtualPodTmpfsMountsDir(virtualSandboxID)
		subPath := strings.TrimPrefix(path, guestpath.SandboxTmpfsMountPrefix)
		return filepath.Join(mountsDir, subPath)
	}
	return SandboxTmpfsMountSource(sandboxID, path)
}

// HugePagesMountSource returns hugepages mount path inside UVM.
func HugePagesMountSource(sandboxID, path string) string {
	mountsDir := HugePagesMountsDir(sandboxID)
	subPath := strings.TrimPrefix(path, guestpath.HugePagesMountPrefix)
	return filepath.Join(mountsDir, subPath)
}

// VirtualPodAwareHugePagesMountSource returns hugepages mount source for virtual pod aware containers.
func VirtualPodAwareHugePagesMountSource(sandboxID, virtualSandboxID, path string) string {
	if virtualSandboxID != "" {
		mountsDir := VirtualPodHugePagesMountsDir(virtualSandboxID)
		subPath := strings.TrimPrefix(path, guestpath.HugePagesMountPrefix)
		return filepath.Join(mountsDir, subPath)
	}
	return HugePagesMountSource(sandboxID, path)
}

// SandboxLogsDir returns the logs directory inside the UVM for forwarding container stdio to.
//
// Virtual pod aware.
func SandboxLogsDir(sandboxID, virtualSandboxID string) string {
	return filepath.Join(VirtualPodAwareSandboxRootDir(sandboxID, virtualSandboxID), "logs")
}

// SandboxLogPath returns the log path inside the UVM.
//
// Virtual pod aware.
func SandboxLogPath(sandboxID, virtualSandboxID, path string) string {
	return filepath.Join(SandboxLogsDir(sandboxID, virtualSandboxID), path)
}

// GetNetworkNamespaceID returns the `ToLower` of
// `spec.Windows.Network.NetworkNamespace` or `""`.
func GetNetworkNamespaceID(spec *oci.Spec) string {
	if spec.Windows != nil &&
		spec.Windows.Network != nil {
		return strings.ToLower(spec.Windows.Network.NetworkNamespace)
	}
	return ""
}

// IsRootReadonly returns `true` if the spec specifies the rootfs is readonly.
func IsRootReadonly(spec *oci.Spec) bool {
	if spec.Root != nil {
		return spec.Root.Readonly
	}
	return false
}

// removeMount removes mount from the array if `target` matches `Destination`
func removeMount(target string, mounts []oci.Mount) []oci.Mount {
	var result []oci.Mount
	for _, m := range mounts {
		if m.Destination == target {
			continue
		}
		result = append(result, m)
	}
	return result
}

func setProcess(spec *oci.Spec) {
	if spec.Process == nil {
		spec.Process = &oci.Process{}
	}
}

func SetCoreRLimit(spec *oci.Spec, value string) error {
	setProcess(spec)

	vals := strings.Split(value, ";")
	if len(vals) != 2 {
		return errors.New("wrong number of values supplied for rlimit core")
	}

	soft, err := strconv.ParseUint(vals[0], 10, 64)
	if err != nil {
		return errors.Wrap(err, "failed to parse soft core rlimit")
	}
	hard, err := strconv.ParseUint(vals[1], 10, 64)
	if err != nil {
		return errors.Wrap(err, "failed to parse hard core rlimit")
	}

	spec.Process.Rlimits = append(spec.Process.Rlimits, oci.POSIXRlimit{
		Type: "RLIMIT_CORE",
		Soft: soft,
		Hard: hard,
	})

	return nil
}

type ParseUserStrResult struct {
	UID       uint32
	GID       uint32
	Username  string
	Groupname string
}

// ParseUserStr parses `userStr`, looks up container filesystem's /etc/passwd and /etc/group
// files for UID and GID for the process.
//
// NB: When `userStr` represents a UID, which doesn't exist, return UID as is with GID set to 0.
func ParseUserStr(rootPath, userStr string) (res ParseUserStrResult, err error) {
	parts := strings.Split(userStr, ":")
	if len(parts) > 2 {
		return res, fmt.Errorf("invalid userstr: '%s'", userStr)
	}

	var v int
	var u user.User
	var g user.Group

	// Handle the "user part" first.
	v, err = strconv.Atoi(parts[0])
	if err != nil {
		// this is a username string, evaluate uid/gid
		u, err = GetUser(rootPath, func(u user.User) bool {
			return u.Name == parts[0]
		})
		if err != nil {
			return res, errors.Wrapf(err, "failed to find user by username: %s", parts[0])
		}
		res.UID = uint32(u.Uid)
		res.GID = uint32(u.Gid)
		res.Username = u.Name
		// Will determine group name below
	} else {
		// this is a UID, parse /etc/passwd to find name and GID.
		u, err = GetUser(rootPath, func(u user.User) bool {
			return u.Uid == v
		})
		if err != nil {
			if OutOfUint32Bounds(v) {
				return res, errors.Errorf("UID (%d) exceeds uint32 bounds", v)
			}
			// UID doesn't exist, continue with GID default to 0 (but we still want to
			// look up its name) unless overridden.
			res.UID = uint32(v)
		} else {
			res.UID = uint32(u.Uid)
			res.GID = uint32(u.Gid)
			res.Username = u.Name
		}
	}

	if len(parts) == 1 {
		g, err = GetGroup(rootPath, func(g user.Group) bool {
			return g.Gid == int(res.GID)
		})
		// If GID doesn't exist we just don't fill in groupName
		if err == nil {
			res.Groupname = g.Name
		}
		return res, nil
	}

	v, err = strconv.Atoi(parts[1])
	if err != nil {
		// this is a group name string, evaluate gid
		g, err = GetGroup(rootPath, func(g user.Group) bool {
			return g.Name == parts[1]
		})
		if err != nil {
			return res, errors.Wrapf(err, "failed to find group by groupname: %s", parts[1])
		}
		res.GID = uint32(g.Gid)
		res.Groupname = g.Name
	} else {
		// this is a GID, parse /etc/group to find name.
		g, err = GetGroup(rootPath, func(g user.Group) bool {
			return g.Gid == v
		})
		if err != nil {
			if OutOfUint32Bounds(v) {
				return res, errors.Errorf("GID (%d) exceeds uint32 bounds", v)
			}
			// GID doesn't exist, continue with GID as is.
			res.GID = uint32(v)
		} else {
			res.GID = uint32(g.Gid)
			res.Groupname = g.Name
		}
	}
	return res, nil
}

// SetUserStr sets `spec.Process` to the valid `userstr` based on the OCI Image Spec
// v1.0.0 `userstr`.
//
// Valid values are: user, uid, user:group, uid:gid, uid:group, user:gid.
// If uid is provided instead of the username then that value is not checked against the
// /etc/passwd file to verify if the user with given uid actually exists.
//
// Since UID and GID are parsed as ints, but will ultimately end up as uint32 in the [OCI spec],
// an error is returned if the the IDs are not within the uint32 bounds ([0, math.MathUint32]).
// This avoid unexpected results if the ID is first parsed as an int and then
// overflows around when downcast (eg, [math.MaxUint32] + 1 will become 0).
// Notes:
//   - Per the [Go spec], we have no indication of overflow when converting between integer types.
//   - "man 5 passwd" and "man 5 group" (as well as [user.ParsePasswdFileFilter] and [user.ParseGroupFilter))
//     do not specify any limits on the UID and GID range.
//
// [OCI spec]: https://pkg.go.dev/github.com/opencontainers/runtime-spec/specs-go#User
// [Go spec]: https://go.dev/ref/spec#Conversions
func SetUserStr(spec *oci.Spec, userstr string) error {
	setProcess(spec)

	usrInfo, err := ParseUserStr(spec.Root.Path, userstr)
	if err != nil {
		return err
	}

	spec.Process.User.UID, spec.Process.User.GID = usrInfo.UID, usrInfo.GID
	return nil
}

// GetUser looks up /etc/passwd file in a container filesystem and returns parsed user
func GetUser(rootPath string, filter func(user.User) bool) (user.User, error) {
	users, err := user.ParsePasswdFileFilter(filepath.Join(rootPath, "/etc/passwd"), filter)
	if err != nil {
		return user.User{}, err
	}
	if len(users) != 1 {
		return user.User{}, errors.Errorf("expected exactly 1 user matched '%d'", len(users))
	}

	if OutOfUint32Bounds(users[0].Uid) {
		return user.User{}, errors.Errorf("UID (%d) exceeds uint32 bounds", users[0].Uid)
	}

	if OutOfUint32Bounds(users[0].Gid) {
		return user.User{}, errors.Errorf("GID (%d) exceeds uint32 bounds", users[0].Gid)
	}
	return users[0], nil
}

// GetGroup looks up /etc/group file in a container filesystem and returns parsed group
func GetGroup(rootPath string, filter func(user.Group) bool) (user.Group, error) {
	groups, err := user.ParseGroupFileFilter(filepath.Join(rootPath, "/etc/group"), filter)
	if err != nil {
		return user.Group{}, err
	}

	if len(groups) != 1 {
		return user.Group{}, errors.Errorf("expected exactly 1 group matched '%d'", len(groups))
	}

	if OutOfUint32Bounds(groups[0].Gid) {
		return user.Group{}, errors.Errorf("GID (%d) exceeds uint32 bounds", groups[0].Gid)
	}
	return groups[0], nil
}

// ApplyAnnotationsToSpec modifies the spec based on additional information from annotations
func ApplyAnnotationsToSpec(ctx context.Context, spec *oci.Spec) error {
	// Check if we need to override container's /dev/shm
	if val, ok := spec.Annotations[annotations.LCOWDevShmSizeInKb]; ok {
		mt, err := devShmMountWithSize(val)
		if err != nil {
			return err
		}
		spec.Mounts = removeMount(devShmPath, spec.Mounts)
		spec.Mounts = append(spec.Mounts, *mt)
		log.G(ctx).WithField("sizeKB", val).Debug("set custom /dev/shm size")
	}

	var err error
	privileged := false
	if val, ok := spec.Annotations[annotations.LCOWPrivileged]; ok {
		privileged, err = strconv.ParseBool(val)
		if err != nil {
			log.G(ctx).WithFields(logrus.Fields{
				logfields.OCIAnnotation: annotations.LCOWPrivileged,
				logfields.Value:         val,
				logfields.ExpectedType:  logfields.Bool,
			}).WithError(err).Warning("annotation value could not be parsed")
		}
	}
	// Check if we need to do any capability/device mappings
	if privileged {
		log.G(ctx).Debugf("'%s' set for privileged container", annotations.LCOWPrivileged)

		// Add all host devices
		hostDevices, err := devices.HostDevices()
		if err != nil {
			return err
		}
		for _, hostDevice := range hostDevices {
			AddLinuxDeviceToSpec(ctx, hostDevice, spec, false)
		}

		// Set the cgroup access
		spec.Linux.Resources.Devices = []oci.LinuxDeviceCgroup{
			{
				Allow:  true,
				Access: "rwm",
			},
		}
	} else {
		tempLinuxDevices := spec.Linux.Devices
		spec.Linux.Devices = []oci.LinuxDevice{}
		for _, ld := range tempLinuxDevices {
			hostDevice, err := devices.DeviceFromPath(ld.Path, "rwm")
			if err != nil {
				return err
			}
			AddLinuxDeviceToSpec(ctx, hostDevice, spec, true)
		}
	}

	return nil
}

// AddDevSev adds SEV device to container spec. On 5.x kernel the device is /dev/sev,
// however this changed in 6.x where the device is /dev/sev-guest.
func AddDevSev(ctx context.Context, spec *oci.Spec) error {
	// try adding /dev/sev, which should be present for 5.x kernel
	devSev, err := devices.DeviceFromPath("/dev/sev", "rwm")
	if err != nil {
		// try adding /dev/guest-sev, which should be present for 6.x kernel
		var errSevGuest error
		devSev, errSevGuest = devices.DeviceFromPath("/dev/sev-guest", "rwm")
		if errSevGuest != nil {
			return fmt.Errorf("failed to add SEV device to spec: %w: %w", err, errSevGuest)
		}
	}
	AddLinuxDeviceToSpec(ctx, devSev, spec, true)
	return nil
}

// devShmMountWithSize returns a /dev/shm device mount with size set to
// `sizeString` if it represents a valid size in KB, returns error otherwise.
func devShmMountWithSize(sizeString string) (*oci.Mount, error) {
	size, err := strconv.ParseUint(sizeString, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("/dev/shm size must be a valid integer: %w", err)
	}
	if size == 0 {
		return nil, errors.New("/dev/shm size must be non-zero")
	}

	// Use the same options as in upstream https://github.com/containerd/containerd/blob/0def98e462706286e6eaeff4a90be22fda75e761/oci/mounts.go#L49
	sizeKB := fmt.Sprintf("size=%sk", sizeString)
	return &oci.Mount{
		Source:      "shm",
		Destination: devShmPath,
		Type:        "tmpfs",
		Options:     []string{"nosuid", "noexec", "nodev", "mode=1777", sizeKB},
	}, nil
}

func OutOfUint32Bounds(v int) bool { return v < 0 || v > math.MaxUint32 }
