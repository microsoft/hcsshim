//go:build linux
// +build linux

package securitypolicy

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	specInternal "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/moby/sys/user"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

//nolint:unused
const osType = "linux"

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

func GetAllUserInfo(process *oci.Process, rootPath string) (
	userIDName IDName,
	groupIDNames []IDName,
	umask string,
	err error,
) {
	passwdPath := filepath.Join(rootPath, "/etc/passwd")
	groupPath := filepath.Join(rootPath, "/etc/group")

	if process == nil {
		err = errors.New("spec.Process is nil")
		return
	}

	// this default value is used in the Linux kernel if no umask is specified
	umask = "0022"
	if process.User.Umask != nil {
		umask = fmt.Sprintf("%04o", *process.User.Umask)
	}

	parsedUserStr := false
	checkGroup := true

	if process.User.Username != "" {
		var usrInfo specInternal.ParseUserStrResult
		usrInfo, err = specInternal.ParseUserStr(rootPath, process.User.Username)
		if err != nil {
			err = errors.Errorf("failed to parse user str %q: %v", process.User.Username, err)
			return
		}
		userIDName = IDName{ID: strconv.FormatUint(uint64(usrInfo.UID), 10), Name: usrInfo.Username}
		groupIDNames = []IDName{
			{ID: strconv.FormatUint(uint64(usrInfo.GID), 10), Name: usrInfo.Groupname},
		}
		parsedUserStr = true
	}

	if !parsedUserStr {
		// fallback UID/GID lookup
		uid := process.User.UID
		userIDName = IDName{ID: strconv.FormatUint(uint64(uid), 10), Name: ""}
		if _, err = os.Stat(passwdPath); err == nil {
			var userInfo user.User
			userInfo, err = specInternal.GetUser(rootPath, func(user user.User) bool {
				return uint32(user.Uid) == uid
			})

			if err != nil {
				return userIDName, nil, "", err
			}

			userIDName.Name = userInfo.Name
		}

		gid := process.User.GID
		groupIDName := IDName{ID: strconv.FormatUint(uint64(gid), 10), Name: ""}

		if _, err = os.Stat(groupPath); err == nil {
			var groupInfo user.Group
			groupInfo, err = specInternal.GetGroup(rootPath, func(group user.Group) bool {
				return uint32(group.Gid) == gid
			})

			if err != nil {
				return userIDName, nil, "", err
			}
			groupIDName.Name = groupInfo.Name
		} else {
			checkGroup = false
		}

		groupIDNames = []IDName{groupIDName}
	}

	additionalGIDs := process.User.AdditionalGids
	if len(additionalGIDs) > 0 {
		for _, gid := range additionalGIDs {
			groupIDName := IDName{ID: strconv.FormatUint(uint64(gid), 10), Name: ""}
			if checkGroup {
				var groupInfo user.Group
				groupInfo, err = specInternal.GetGroup(rootPath, func(group user.Group) bool {
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
