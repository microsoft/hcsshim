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

// SandboxMountsDir returns sandbox mounts directory inside UVM/host.
func SandboxMountsDir(sandboxID string) string {
	return filepath.Join(SandboxRootDir(sandboxID), "sandboxMounts")
}

// HugePagesMountsDir returns hugepages mounts directory inside UVM.
func HugePagesMountsDir(sandboxID string) string {
	return filepath.Join(SandboxRootDir(sandboxID), "hugepages")
}

// SandboxMountSource returns sandbox mount path inside UVM
func SandboxMountSource(sandboxID, path string) string {
	mountsDir := SandboxMountsDir(sandboxID)
	subPath := strings.TrimPrefix(path, guestpath.SandboxMountPrefix)
	return filepath.Join(mountsDir, subPath)
}

// HugePagesMountSource returns hugepages mount path inside UVM
func HugePagesMountSource(sandboxID, path string) string {
	mountsDir := HugePagesMountsDir(sandboxID)
	subPath := strings.TrimPrefix(path, guestpath.HugePagesMountPrefix)
	return filepath.Join(mountsDir, subPath)
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

// ParseUserStr parses `userStr`, looks up container filesystem's /etc/passwd and /etc/group
// files for UID and GID for the process.
//
// NB: When `userStr` represents a UID, which doesn't exist, return UID as is with GID set to 0.
func ParseUserStr(rootPath, userStr string) (uint32, uint32, error) {
	parts := strings.Split(userStr, ":")
	switch len(parts) {
	case 1:
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			// this is a username string, evaluate uid/gid
			u, err := getUser(rootPath, func(u user.User) bool {
				return u.Name == userStr
			})
			if err != nil {
				return 0, 0, errors.Wrapf(err, "failed to find user by username: %s", userStr)
			}
			return uint32(u.Uid), uint32(u.Gid), nil
		}

		// this is a UID, parse /etc/passwd to find GID.
		u, err := getUser(rootPath, func(u user.User) bool {
			return u.Uid == v
		})
		if err != nil {
			// UID doesn't exist, return as is with GID 0.
			if OutOfUint32Bounds(v) {
				return 0, 0, errors.Errorf("UID (%d) exceeds uint32 bounds", v)
			}
			return uint32(v), 0, nil
		}
		return uint32(u.Uid), uint32(u.Gid), nil
	case 2:
		var (
			userName, groupName string
			uid, gid            uint32
		)
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			// username string, lookup UID
			userName = parts[0]
			u, err := getUser(rootPath, func(u user.User) bool {
				return u.Name == userName
			})
			if err != nil {
				return 0, 0, errors.Wrapf(err, "failed to find user by username: %s", userName)
			}
			uid = uint32(u.Uid)
		} else {
			if OutOfUint32Bounds(v) {
				return 0, 0, errors.Errorf("UID (%d) exceeds uint32 bounds", v)
			}
			uid = uint32(v)
		}

		v, err = strconv.Atoi(parts[1])
		if err != nil {
			// groupname string, lookup GID
			groupName = parts[1]
			g, err := getGroup(rootPath, func(g user.Group) bool {
				return g.Name == groupName
			})
			if err != nil {
				return 0, 0, errors.Wrapf(err, "failed to find group by groupname: %s", groupName)
			}
			gid = uint32(g.Gid)
		} else {
			if OutOfUint32Bounds(v) {
				return 0, 0, errors.Errorf("GID (%d) exceeds uint32 bounds", v)
			}
			gid = uint32(v)
		}
		return uid, gid, nil
	default:
		return 0, 0, fmt.Errorf("invalid userstr: '%s'", userStr)
	}
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

	uid, gid, err := ParseUserStr(spec.Root.Path, userstr)
	if err != nil {
		return err
	}

	spec.Process.User.UID, spec.Process.User.GID = uid, gid
	return nil
}

// getUser looks up /etc/passwd file in a container filesystem and returns parsed user
func getUser(rootPath string, filter func(user.User) bool) (user.User, error) {
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

// getGroup looks up /etc/group file in a container filesystem and returns parsed group
func getGroup(rootPath string, filter func(user.Group) bool) (user.Group, error) {
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
