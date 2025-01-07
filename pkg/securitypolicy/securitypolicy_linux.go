//go:build linux
// +build linux

package securitypolicy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	specGuest "github.com/Microsoft/hcsshim/internal/guest/spec"
	specInternal "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/moby/sys/user"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

// This is being used by StandEnforcer.
// substituteUVMPath substitutes mount prefix to an appropriate path inside
// UVM. At policy generation time, it's impossible to tell what the sandboxID
// will be, so the prefix substitution needs to happen during runtime.
func substituteUVMPath(sandboxID string, m mountInternal) mountInternal {
	if strings.HasPrefix(m.Source, guestpath.SandboxMountPrefix) {
		m.Source = specInternal.SandboxMountSource(sandboxID, m.Source)
	} else if strings.HasPrefix(m.Source, guestpath.HugePagesMountPrefix) {
		m.Source = specInternal.HugePagesMountSource(sandboxID, m.Source)
	}
	return m
}

// SandboxMountsDir returns sandbox mounts directory inside UVM/host.
func SandboxMountsDir(sandboxID string) string {
	return specInternal.SandboxMountsDir((sandboxID))
}

// HugePagesMountsDir returns hugepages mounts directory inside UVM.
func HugePagesMountsDir(sandboxID string) string {
	return specInternal.HugePagesMountsDir(sandboxID)
}

func getUser(passwdPath string, filter func(user.User) bool) (user.User, error) {
	users, err := user.ParsePasswdFileFilter(passwdPath, filter)
	if err != nil {
		return user.User{}, err
	}
	if len(users) != 1 {
		return user.User{}, errors.Errorf("expected exactly 1 user matched '%d'", len(users))
	}
	return users[0], nil
}

func getGroup(groupPath string, filter func(user.Group) bool) (user.Group, error) {
	groups, err := user.ParseGroupFileFilter(groupPath, filter)
	if err != nil {
		return user.Group{}, err
	}
	if len(groups) != 1 {
		return user.Group{}, errors.Errorf("expected exactly 1 group matched '%d'", len(groups))
	}
	return groups[0], nil
}

func GetAllUserInfo(containerID string, process *oci.Process) (IDName, []IDName, string, error) {
	rootPath := filepath.Join(guestpath.LCOWRootPrefixInUVM, containerID, guestpath.RootfsPath)
	passwdPath := filepath.Join(rootPath, "/etc/passwd")
	groupPath := filepath.Join(rootPath, "/etc/group")

	if process == nil {
		return IDName{}, nil, "", errors.New("spec.Process is nil")
	}

	// this default value is used in the Linux kernel if no umask is specified
	umask := "0022"
	if process.User.Umask != nil {
		umask = fmt.Sprintf("%04o", *process.User.Umask)
	}

	if process.User.Username != "" {
		uid, gid, err := specGuest.ParseUserStr(rootPath, process.User.Username)
		if err == nil {
			userIDName := IDName{ID: strconv.FormatUint(uint64(uid), 10)}
			groupIDName := IDName{ID: strconv.FormatUint(uint64(gid), 10)}
			return userIDName, []IDName{groupIDName}, umask, nil
		}
		log.G(context.Background()).WithError(err).Warn("failed to parse user str, fallback to lookup")
	}

	// fallback UID/GID lookup
	uid := process.User.UID
	userIDName := IDName{ID: strconv.FormatUint(uint64(uid), 10), Name: ""}
	if _, err := os.Stat(passwdPath); err == nil {
		userInfo, err := getUser(passwdPath, func(user user.User) bool {
			return uint32(user.Uid) == uid
		})

		if err != nil {
			return userIDName, nil, "", err
		}

		userIDName.Name = userInfo.Name
	}

	gid := process.User.GID
	groupIDName := IDName{ID: strconv.FormatUint(uint64(gid), 10), Name: ""}

	checkGroup := true
	if _, err := os.Stat(groupPath); err == nil {
		groupInfo, err := getGroup(groupPath, func(group user.Group) bool {
			return uint32(group.Gid) == gid
		})

		if err != nil {
			return userIDName, nil, "", err
		}
		groupIDName.Name = groupInfo.Name
	} else {
		checkGroup = false
	}

	groupIDNames := []IDName{groupIDName}
	additionalGIDs := process.User.AdditionalGids
	if len(additionalGIDs) > 0 {
		for _, gid := range additionalGIDs {
			groupIDName = IDName{ID: strconv.FormatUint(uint64(gid), 10), Name: ""}
			if checkGroup {
				groupInfo, err := getGroup(groupPath, func(group user.Group) bool {
					return uint32(group.Gid) == gid
				})
				if err != nil {
					return userIDName, nil, "", err
				}
				groupIDName.Name = groupInfo.Name
			}
			groupIDNames = append(groupIDNames, groupIDName)
		}
	}

	return userIDName, groupIDNames, umask, nil
}
