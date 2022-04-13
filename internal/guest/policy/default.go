//go:build linux
// +build linux

package policy

import (
	oci "github.com/opencontainers/runtime-spec/specs-go"

	internalSpec "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

func ExtendPolicyWithNetworkingMounts(sandboxID string, enforcer securitypolicy.SecurityPolicyEnforcer, spec *oci.Spec) error {
	roSpec := &oci.Spec{
		Root: spec.Root,
	}
	networkingMounts := internalSpec.GenerateWorkloadContainerNetworkMounts(sandboxID, roSpec)
	if err := enforcer.ExtendDefaultMounts(networkingMounts); err != nil {
		return err
	}
	return nil
}

// DefaultCRIMounts returns default mounts added to linux spec by containerD.
func DefaultCRIMounts() []oci.Mount {
	return []oci.Mount{
		{
			Destination: "/proc",
			Type:        "proc",
			Source:      "proc",
			Options:     []string{"nosuid", "noexec", "nodev"},
		},
		{
			Destination: "/dev",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
		},
		{
			Destination: "/dev/pts",
			Type:        "devpts",
			Source:      "devpts",
			Options:     []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620", "gid=5"},
		},
		{
			Destination: "/dev/shm",
			Type:        "tmpfs",
			Source:      "shm",
			Options:     []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"},
		},
		{
			Destination: "/dev/mqueue",
			Type:        "mqueue",
			Source:      "mqueue",
			Options:     []string{"nosuid", "noexec", "nodev"},
		},
		{
			Destination: "/sys",
			Type:        "sysfs",
			Source:      "sysfs",
			Options:     []string{"nosuid", "noexec", "nodev", "ro"},
		},
		{
			Destination: "/run",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
		},
		// cgroup mount is always added by default, regardless if it is present
		// in the mount constraints or not. If the user chooses to override it,
		// then a corresponding mount constraint should be present.
		{
			Source:      "cgroup",
			Destination: "/sys/fs/cgroup",
			Type:        "cgroup",
			Options:     []string{"nosuid", "noexec", "nodev", "relatime", "ro"},
		},
	}
}
