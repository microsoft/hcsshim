//go:build windows && wcowprocess

package processisolated

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/winapi"

	"github.com/Microsoft/go-winio/pkg/guid"
	"golang.org/x/sys/windows"
)

// groupExists reports whether a local group with the given name exists.
func groupExists(groupName string) bool {
	var p *byte
	if err := winapi.NetLocalGroupGetInfo("", groupName, 1, &p); err != nil {
		return false
	}
	_ = windows.NetApiBufferFree(p)
	return true
}

// makeLocalAccount creates a local account with a random password and adds it to groupName.
func makeLocalAccount(ctx context.Context, user, groupName string) (err error) {
	g, err := guid.NewV4()
	if err != nil {
		return fmt.Errorf("generate password: %w", err)
	}
	pswd, err := windows.UTF16PtrFromString(g.String())
	if err != nil {
		return err
	}
	userUTF16, err := windows.UTF16PtrFromString(user)
	if err != nil {
		return err
	}

	usr1 := &winapi.UserInfo1{
		Name:     userUTF16,
		Password: pswd,
		Priv:     winapi.USER_PRIV_USER,
		Flags:    winapi.UF_NORMAL_ACCOUNT | winapi.UF_DONT_EXPIRE_PASSWD,
	}
	if err := winapi.NetUserAdd("", 1, (*byte)(unsafe.Pointer(usr1)), nil); err != nil {
		return fmt.Errorf("create user %s: %w", user, err)
	}
	defer func() {
		if err != nil {
			_ = winapi.NetUserDel("", user)
		}
	}()

	log.G(ctx).WithField("username", user).Debug("created local user account for host process container")

	sid, _, _, err := windows.LookupSID("", user)
	if err != nil {
		return fmt.Errorf("lookup SID for user %q: %w", user, err)
	}
	sids := []winapi.LocalGroupMembersInfo0{{Sid: sid}}
	if err := winapi.NetLocalGroupAddMembers("", groupName, 0, (*byte)(unsafe.Pointer(&sids[0])), 1); err != nil {
		return fmt.Errorf("add user %q to group %q: %w", user, groupName, err)
	}
	return nil
}

// setupHostProcessUser creates an ephemeral local user when userOrGroup names a
// local group, adds it to that group, and returns the created username. When
// userOrGroup is a real user it returns an empty string and no account is created.
func (c *Controller) setupHostProcessUser(ctx context.Context, userOrGroup string) (string, error) {
	if !groupExists(userOrGroup) {
		return "", nil
	}
	userName := c.containerID
	if len(userName) > winapi.UserNameCharLimit {
		userName = userName[:winapi.UserNameCharLimit]
	}
	if err := makeLocalAccount(ctx, userName, userOrGroup); err != nil {
		return "", err
	}
	return userName, nil
}
