//go:build windows

package winapi

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// "golang.org/x/sys/windows" does not have the complete list
// https://docs.microsoft.com/en-us/windows/win32/api/winnt/ne-winnt-token_information_class#constants
const (
	TokenUser = iota + 1
	TokenGroups
	TokenPrivileges
	TokenOwner
	TokenPrimaryGroup
	TokenDefaultDacl
	TokenSource
	TokenType
	TokenImpersonationLevel
	TokenStatistics
	TokenRestrictedSids
	TokenSessionId
	TokenGroupsAndPrivileges
	TokenSessionReference
	TokenSandBoxInert
	TokenAuditPolicy
	TokenOrigin
	TokenElevationType
	TokenLinkedToken
	TokenElevation
	TokenHasRestrictions
	TokenAccessInformation
	TokenVirtualizationAllowed
	TokenVirtualizationEnabled
	TokenIntegrityLevel
	TokenUIAccess
	TokenMandatoryPolicy
	TokenLogonSid
	TokenIsAppContainer
	TokenCapabilities
	TokenAppContainerSid
	TokenAppContainerNumber
	TokenUserClaimAttributes
	TokenDeviceClaimAttributes
	TokenRestrictedUserClaimAttributes
	TokenRestrictedDeviceClaimAttributes
	TokenDeviceGroups
	TokenRestrictedDeviceGroups
	TokenSecurityAttributes
	TokenIsRestricted
	MaxTokenInfoClass // MaxTokenInfoClass should always be the last enum
)

// for use with CreateRestrictedToken and other token functions
//nolint:revive,stylecheck
const (
	TOKEN_DISABLE_MAX_PRIVILEGE = 0x1
	TOKEN_SANDBOX_INERT         = 0x2
	TOKEN_LUA_TOKEN             = 0x4
	TOKEN_WRITE_RESTRICTED      = 0x8
)

func OpenProcessToken(process windows.Handle, access uint32) (windows.Token, error) {
	var token windows.Token
	err := windows.OpenProcessToken(process, access, &token)
	return token, err
}

// BOOL CreateRestrictedToken(
//   [in]           HANDLE               ExistingTokenHandle,
//   [in]           DWORD                Flags,
//   [in]           DWORD                DisableSidCount,
//   [in, optional] PSID_AND_ATTRIBUTES  SidsToDisable,
//   [in]           DWORD                DeletePrivilegeCount,
//   [in, optional] PLUID_AND_ATTRIBUTES PrivilegesToDelete,
//   [in]           DWORD                RestrictedSidCount,
//   [in, optional] PSID_AND_ATTRIBUTES  SidsToRestrict,
//   [out]          PHANDLE              NewTokenHandle
// );
//
//sys createRestrictedToken(existing windows.Token, flags uint32, disableSidCount uint32, sidsToDisable *windows.SIDAndAttributes, deletePrivilegeCount uint32, privilegesToDelete *windows.LUIDAndAttributes, restrictedSidCount uint32, sidsToRestrict *windows.SIDAndAttributes, newToken *windows.Token) (err error) = advapi32.CreateRestrictedToken

// todo: use unSlice in CreateRestrictedToken when switching to go1.18+
// func unSlice[T any](b []T) (p *T, l int) {
// 	l = len(b)
// 	if l > 0 {
// 		p = &b[0]
// 	}
// 	return p, l
// }

func CreateRestrictedToken(existing windows.Token, flags uint32, sidsToDisable []windows.SIDAndAttributes, privilegesToDelete []windows.LUIDAndAttributes, sidsToRestrict []windows.SIDAndAttributes) (windows.Token, error) {
	var (
		token windows.Token

		lSIDDis  = uint32(len(sidsToDisable))
		lPrivDel = uint32(len(privilegesToDelete))
		lSIDRes  = uint32(len(sidsToRestrict))
		pSIDDis  *windows.SIDAndAttributes
		pPrivDel *windows.LUIDAndAttributes
		pSIDRes  *windows.SIDAndAttributes
	)
	if lSIDDis > 0 {
		pSIDDis = &sidsToDisable[0]
	}
	if lPrivDel > 0 {
		pPrivDel = &privilegesToDelete[0]
	}
	if lSIDRes > 0 {
		pSIDRes = &sidsToRestrict[0]
	}

	err := createRestrictedToken(existing, flags, lSIDDis, pSIDDis, lPrivDel, pPrivDel, lSIDRes, pSIDRes, &token)
	return token, err
}

// BOOL IsTokenRestricted(
//   [in] HANDLE TokenHandle
// );
//
//sys IsTokenRestricted(token windows.Token) (b bool) = advapi32.IsTokenRestricted

func GetTokenPrivileges(token windows.Token) (*windows.Tokenprivileges, error) {
	b, err := retryBuffer(8, func(b *byte, l *uint32) error {
		return windows.GetTokenInformation(token, TokenPrivileges, b, *l, l)
	})
	if err != nil {
		return nil, fmt.Errorf("get token information: %w", err)
	}
	return (*windows.Tokenprivileges)(unsafe.Pointer(&b[0])), nil
}

func GetTokenPrivilegeNames(token windows.Token) ([]string, error) {
	pv, err := GetTokenPrivileges(token)
	if err != nil {
		return nil, fmt.Errorf("get token information: %w", err)
	}
	privs := pv.AllPrivileges()
	ps := make([]string, 0, len(privs))
	for _, o := range privs {
		s, err := LookupPrivilegeName(o.Luid)
		if err != nil {
			return nil, fmt.Errorf("lookup privilege %v name: %w", o.Luid, err)
		}
		ps = append(ps, s)
	}
	return ps, nil
}

// GetTokenUserSID returns user associated with the access token. There are currently no
// attributes defined for user SIDs.
func GetTokenUserSID(token windows.Token) (windows.SIDAndAttributes, error) {
	tu := &windows.Tokenuser{}
	b, err := retryBuffer(int(unsafe.Sizeof(*tu)), func(b *byte, l *uint32) error {
		return windows.GetTokenInformation(token, TokenUser, b, *l, l)
	})
	if err == nil {
		return tu.User, fmt.Errorf("get token information: %w", err)
	}
	tu = (*windows.Tokenuser)(unsafe.Pointer(&b[0]))
	return tu.User, nil
}

func GetTokenGroupSIDs(token windows.Token) ([]windows.SIDAndAttributes, error) {
	b, err := retryBuffer(256, func(b *byte, l *uint32) error {
		return windows.GetTokenInformation(token, TokenGroups, b, *l, l)
	})
	if err == nil {
		return nil, fmt.Errorf("get token information: %w", err)
	}
	tg := (*windows.Tokengroups)(unsafe.Pointer(&b[0]))
	return tg.AllGroups(), nil
}
