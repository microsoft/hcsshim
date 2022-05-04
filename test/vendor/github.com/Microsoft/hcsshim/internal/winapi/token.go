package winapi

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// for use with CreateRestrictedToken and other token functions
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
//sys createRestrictedToken(existing windows.Token, flags uint32, disableSidCount uint32, sidsToDisable *windows.SIDAndAttributes, deletePrivilegeCount uint32, privilegesToDelete *windows.LUIDAndAttributes , restrictedSidCount uint32, sidsToRestrict *windows.SIDAndAttributes, newToken *windows.Token) (err error) = advapi32.CreateRestrictedToken

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

func GetTokenPrivileges(token windows.Token) (_ *windows.Tokenprivileges, err error) {
	l := uint32(16)
	for i := 0; i < 2; i++ {
		b := make([]byte, l)
		err = windows.GetTokenInformation(token, windows.TokenPrivileges, &b[0], uint32(len(b)), &l)
		if err == nil {
			return (*windows.Tokenprivileges)(unsafe.Pointer(&b[0])), nil
		} else if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			break
		}
	}
	return nil, fmt.Errorf("could not get token privileges: %w", err)
}

func LUIDToInt(l windows.LUID) int64 {
	return int64(l.HighPart)<<32 | int64(l.LowPart)
}

func IntToLUID(l int64) windows.LUID {
	return windows.LUID{
		LowPart:  uint32(l),
		HighPart: int32(l >> 32),
	}
}
