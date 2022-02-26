package jobcontainers

import (
	"context"
	"fmt"
	"strings"
	"unsafe"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

func randomPswd() (*uint16, error) {
	g, err := guid.NewV4()
	if err != nil {
		return nil, err
	}
	return windows.UTF16PtrFromString(g.String())
}

func groupExists(groupName string) bool {
	var p *byte
	if err := winapi.NetLocalGroupGetInfo(
		"",
		groupName,
		1,
		&p,
	); err != nil {
		return false
	}
	defer windows.NetApiBufferFree(p)
	return true
}

// makeLocalAccount creates a local account with the passed in username and a randomly generated password.
// The user specified by `user`` will added to the `groupName`. This function does not check if groupName exists, that must be handled
// the caller.
func makeLocalAccount(ctx context.Context, user, groupName string) (_ *uint16, err error) {
	// Create a local account with a random password
	pswd, err := randomPswd()
	if err != nil {
		return nil, fmt.Errorf("failed to generate random password: %w", err)
	}

	userUTF16, err := windows.UTF16PtrFromString(user)
	if err != nil {
		return nil, fmt.Errorf("failed to encode username to UTF16: %w", err)
	}

	usr1 := &winapi.UserInfo1{
		Name:     userUTF16,
		Password: pswd,
		Priv:     winapi.USER_PRIV_USER,
		Flags:    winapi.UF_NORMAL_ACCOUNT | winapi.UF_DONT_EXPIRE_PASSWD,
	}
	if err := winapi.NetUserAdd(
		"",
		1,
		(*byte)(unsafe.Pointer(usr1)),
		nil,
	); err != nil {
		return nil, fmt.Errorf("failed to create user %s: %w", user, err)
	}
	defer func() {
		if err != nil {
			_ = winapi.NetUserDel("", user)
		}
	}()

	log.G(ctx).WithField("username", user).Debug("Created local user account for job container")

	sid, _, _, err := windows.LookupSID("", user)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup SID for user %q: %w", user, err)
	}

	sids := []winapi.LocalGroupMembersInfo0{{Sid: sid}}
	if err := winapi.NetLocalGroupAddMembers(
		"",
		groupName,
		0,
		(*byte)(unsafe.Pointer(&sids[0])),
		1,
	); err != nil {
		return nil, fmt.Errorf("failed to add user %q to the %q group: %w", user, groupName, err)
	}

	return pswd, nil
}

// processToken verifies first whether userOrGroup is a username or group name. If it's a valid group name,
// a temporary local user account will be created and added to the group and then the token for the user will
// be returned. If it is not a group name then the user will logged into and the token will be returned.
func (c *JobContainer) processToken(ctx context.Context, userOrGroup string) (windows.Token, error) {
	var (
		domain   string
		userName string
		token    windows.Token
	)

	if userOrGroup == "" {
		return 0, errors.New("empty username or group name passed")
	}

	if groupExists(userOrGroup) {
		userName = c.id[:winapi.UserNameCharLimit]
		pswd, err := makeLocalAccount(ctx, userName, userOrGroup)
		if err != nil {
			return 0, fmt.Errorf("failed to create local account for container: %w", err)
		}
		if err := winapi.LogonUser(
			windows.StringToUTF16Ptr(userName),
			nil,
			pswd,
			winapi.LOGON32_LOGON_INTERACTIVE,
			winapi.LOGON32_PROVIDER_DEFAULT,
			&token,
		); err != nil {
			return 0, fmt.Errorf("failed to logon user: %w", err)
		}
		c.localUserAccount = userName
		return token, nil
	}

	// Must be a user string, split it by domain and username
	split := strings.Split(userOrGroup, "\\")
	if len(split) == 2 {
		domain = split[0]
		userName = split[1]
	} else if len(split) == 1 {
		userName = split[0]
	} else {
		return 0, fmt.Errorf("invalid user string `%s`", userOrGroup)
	}

	logonType := winapi.LOGON32_LOGON_INTERACTIVE
	// User asking to run as a local system account (NETWORK SERVICE, LOCAL SERVICE, SYSTEM)
	if domain == "NT AUTHORITY" {
		logonType = winapi.LOGON32_LOGON_SERVICE
	}

	if err := winapi.LogonUser(
		windows.StringToUTF16Ptr(userName),
		windows.StringToUTF16Ptr(domain),
		nil,
		logonType,
		winapi.LOGON32_PROVIDER_DEFAULT,
		&token,
	); err != nil {
		return 0, fmt.Errorf("failed to logon user: %w", err)
	}
	return token, nil
}

func openCurrentProcessToken() (windows.Token, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ALL_ACCESS, &token); err != nil {
		return 0, errors.Wrap(err, "failed to open current process token")
	}
	return token, nil
}
