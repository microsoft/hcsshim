//go:build windows

package winapi

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// for use with CreateRestrictedToken and other token functions
//nolint:revive,stylecheck
const (
	TOKEN_DISABLE_MAX_PRIVILEGE = 0x1
	TOKEN_SANDBOX_INERT         = 0x2
	TOKEN_LUA_TOKEN             = 0x4
	TOKEN_WRITE_RESTRICTED      = 0x8
)

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
//sys createRestrictedToken(existing windows.Token, flags uint32, disableSidCount uint32, sidsToDisable *windows.SIDAndAttributes, deletePrivilegeCount uint32, privilegesToDelete *windows.LUIDAndAttributes , restrictedSidCount uint32, sidsToRestrict *windows.SIDAndAttributes, newToken *windows.Token) (err error) = advapi32.CreateRestrictedToken

// todo: use unslice in CreateRestrictedToken when switching to go1.18+
// func unslice[T any](b []T) (p *T, l int) {
// 	l = len(b)
// 	if l > 0 {
// 		p = &b[0]
// 	}
// 	return p, l
// }

func CreateRestrictedToken(existing windows.Token, flags uint32, sidsToDisable []windows.SIDAndAttributes, privilegesToDelete []windows.LUIDAndAttributes, sidsToRestrict []windows.SIDAndAttributes, newToken *windows.Token) (err error) {
	var (
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

	return createRestrictedToken(existing, flags, lSIDDis, pSIDDis, lPrivDel, pPrivDel, lSIDRes, pSIDRes, newToken)
}

func GetTokenPrivileges(token windows.Token) (*windows.Tokenprivileges, error) {
	b, err := retryBuffer(8, func(b *byte, l *uint32) error {
		return windows.GetTokenInformation(token, windows.TokenPrivileges, b, *l, l)
	})
	if err == nil {
		return (*windows.Tokenprivileges)(unsafe.Pointer(&b[0])), nil
	}
	return nil, fmt.Errorf("could not get token privileges: %w", err)
}

func GetTokenPrivilegeNames(token windows.Token) []string {
	ps := make([]string, 0)
	pv, err := GetTokenPrivileges(token)
	if err == nil {
		for _, o := range pv.AllPrivileges() {
			if s, err := LookupPrivilegeName(o.Luid); err == nil {
				ps = append(ps, fmt.Sprintf("%s [%d]", s, o.Attributes))
			}
		}
	}
	return ps
}
