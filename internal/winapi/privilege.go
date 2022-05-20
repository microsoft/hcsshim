//go:build windows

package winapi

import (
	"fmt"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

const (
	SeChangeNotifyPrivilege = "SeChangeNotifyPrivilege"
	SeBackupPrivilege       = winio.SeBackupPrivilege
	SeRestorePrivilege      = winio.SeRestorePrivilege
	SeCreateGlobalPrivilege = "SeCreateGlobalPrivilege"
	SeManageVolumePrivilege = "SeManageVolumePrivilege"
)

func LookupPrivilegeValue(p string) (l windows.LUID, err error) {
	err = windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr(p), &l)
	return l, err
}

// BOOL LookupPrivilegeNameW(
//   [in, optional]  LPCWSTR lpSystemName,
//   [in]            PLUID   lpLuid,
//   [out, optional] LPWSTR  lpName,
//   [in, out]       LPDWORD cchName
// );
//
//sys lookupPrivilegeName(systemName string, luid *windows.LUID, buffer *uint16, size *uint32) (err error) = advapi32.LookupPrivilegeNameW

func LookupPrivilegeName(luid windows.LUID) (string, error) {
	s, err := retryLStr(-2, func(b *uint16, l *uint32) error {
		return lookupPrivilegeName("", &luid, b, l)
	})
	if err != nil {
		return "", fmt.Errorf("could not lookup LUID %v: %w", luid, err)
	}
	return windows.UTF16ToString(s), nil
}

// BOOL LookupPrivilegeDisplayNameW(
//   [in, optional]  LPCWSTR lpSystemName,
//   [in]            LPCWSTR lpName,
//   [out, optional] LPWSTR  lpDisplayName,
//   [in, out]       LPDWORD cchDisplayName,
//   [out]           LPDWORD lpLanguageId
// );
//
//sys lookupPrivilegeDisplayName(systemName string, name string, buffer *uint16, size *uint32, languageId *uint32) (err error) = advapi32.LookupPrivilegeDisplayNameW

func LookupPrivilegeDisplayName(s string) (string, error) {
	var langID uint32
	ss, err := retryLStr(0, func(b *uint16, l *uint32) error {
		return lookupPrivilegeDisplayName("", s, b, l, &langID)
	})
	if err != nil {
		return "", fmt.Errorf("could not lookup privilege %s: %w", s, err)
	}
	return windows.UTF16ToString(ss), nil
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
