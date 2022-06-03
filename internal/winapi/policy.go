//go:build windows

package winapi

import (
	"fmt"
	"math"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

//nolint:revive,stylecheck
const (
	LSA_POLICY_VIEW_LOCAL_INFORMATION   = 0x00000001
	LSA_POLICY_VIEW_AUDIT_INFORMATION   = 0x00000002
	LSA_POLICY_GET_PRIVATE_INFORMATION  = 0x00000004
	LSA_POLICY_TRUST_ADMIN              = 0x00000008
	LSA_POLICY_CREATE_ACCOUNT           = 0x00000010
	LSA_POLICY_CREATE_SECRET            = 0x00000020
	LSA_POLICY_CREATE_PRIVILEGE         = 0x00000040
	LSA_POLICY_SET_DEFAULT_QUOTA_LIMITS = 0x00000080
	LSA_POLICY_SET_AUDIT_REQUIREMENTS   = 0x00000100
	LSA_POLICY_AUDIT_LOG_ADMIN          = 0x00000200
	LSA_POLICY_SERVER_ADMIN             = 0x00000400
	LSA_POLICY_LOOKUP_NAMES             = 0x00000800

	LSA_POLICY_ALL_ACCESS = windows.STANDARD_RIGHTS_REQUIRED |
		LSA_POLICY_VIEW_LOCAL_INFORMATION |
		LSA_POLICY_VIEW_AUDIT_INFORMATION |
		LSA_POLICY_GET_PRIVATE_INFORMATION |
		LSA_POLICY_TRUST_ADMIN |
		LSA_POLICY_CREATE_ACCOUNT |
		LSA_POLICY_CREATE_SECRET |
		LSA_POLICY_CREATE_PRIVILEGE |
		LSA_POLICY_SET_DEFAULT_QUOTA_LIMITS |
		LSA_POLICY_SET_AUDIT_REQUIREMENTS |
		LSA_POLICY_AUDIT_LOG_ADMIN |
		LSA_POLICY_SERVER_ADMIN |
		LSA_POLICY_LOOKUP_NAMES

	LSA_POLICY_READ = windows.STANDARD_RIGHTS_READ |
		LSA_POLICY_VIEW_AUDIT_INFORMATION |
		LSA_POLICY_GET_PRIVATE_INFORMATION

	LSA_POLICY_WRITE = windows.STANDARD_RIGHTS_WRITE |
		LSA_POLICY_TRUST_ADMIN |
		LSA_POLICY_CREATE_ACCOUNT |
		LSA_POLICY_CREATE_SECRET |
		LSA_POLICY_CREATE_PRIVILEGE |
		LSA_POLICY_SET_DEFAULT_QUOTA_LIMITS |
		LSA_POLICY_SET_AUDIT_REQUIREMENTS |
		LSA_POLICY_AUDIT_LOG_ADMIN |
		LSA_POLICY_SERVER_ADMIN

	LSA_POLICY_EXECUTE = windows.STANDARD_RIGHTS_EXECUTE |
		LSA_POLICY_VIEW_LOCAL_INFORMATION |
		LSA_POLICY_LOOKUP_NAMES
)

type LSAHandle uintptr

// typedef struct _LSA_UNICODE_STRING {
//   USHORT Length;
//   USHORT MaximumLength;
//   PWSTR  Buffer;
// } LSA_UNICODE_STRING, *PLSA_UNICODE_STRING;

type LSAUnicodeString struct {
	// Specifies the length, in bytes, of the string pointed to by the Buffer member,
	// not including the terminating null character, if any.
	Length uint16
	// Specifies the total size, in bytes, of the memory allocated for Buffer.
	// Up to MaximumLength bytes can be written into the buffer without trampling memory.
	MaximumLength uint16
	// May not be null-terminated.
	Buffer *uint16
}

func LSAUnicodeStringFromString(s string) (LSAUnicodeString, error) {
	us := LSAUnicodeString{}
	p, err := windows.UTF16FromString(s)
	if err != nil {
		return us, err
	}
	if len(p) >= math.MaxUint16 {
		return us, windows.ERROR_INSUFFICIENT_BUFFER
	}
	lp := uint16(len(p))
	szU16 := uint16(unsafe.Sizeof(uint16(0)))
	// get rid of null byte
	us.Length = (lp - 1) * szU16
	us.MaximumLength = lp * szU16
	if lp > 0 {
		us.Buffer = &p[0]
	}
	return us, nil
}

// typedef struct _LSA_OBJECT_ATTRIBUTES {
//   ULONG               Length;
//   HANDLE              RootDirectory;
//   PLSA_UNICODE_STRING ObjectName;
//   ULONG               Attributes;
//   PVOID               SecurityDescriptor;
//   PVOID               SecurityQualityOfService;
// } LSA_OBJECT_ATTRIBUTES, *PLSA_OBJECT_ATTRIBUTES;

type LSAObjectAttributes struct {
	Length                   uint32
	RootDirectory            windows.Handle
	ObjectName               *LSAUnicodeString
	Attributes               uint32
	SecurityDescriptor       uint32
	SecurityQualityOfService uint32
}

// NTSTATUS LsaOpenPolicy(
//   [in]      PLSA_UNICODE_STRING    SystemName,
//   [in]      PLSA_OBJECT_ATTRIBUTES ObjectAttributes,
//   [in]      ACCESS_MASK            DesiredAccess,
//   [in, out] PLSA_HANDLE            PolicyHandle
// );
//
//sys lsaOpenPolicy(systemName *LSAUnicodeString, attributes *LSAObjectAttributes, access uint32, policy *LSAHandle) (ntstatus error) = advapi32.LsaOpenPolicy

func LSAOpenPolicy(access uint32) (LSAHandle, error) {
	var h LSAHandle
	attr := LSAObjectAttributes{}
	if err := lsaOpenPolicy(nil, &attr, access, &h); err != nil {
		return LSAHandle(windows.InvalidHandle), err
	}
	return h, nil
}

// NTSTATUS LsaClose(
//   [in] LSA_HANDLE ObjectHandle
// );
//
//sys LSAClose(policy LSAHandle) (ntstatus error) = advapi32.LsaClose

func LSAAddAccountRightsString(policy LSAHandle, account *windows.SID, privileges []string) error {
	rights := make([]LSAUnicodeString, 0, len(privileges))
	for _, p := range privileges {
		pp, err := LSAUnicodeStringFromString(p)
		if err != nil {
			return fmt.Errorf("convert privilege name %q to UTF-16 string: %w", p, err)
		}
		rights = append(rights, pp)
	}
	return LSAAddAccountRights(policy, account, rights)
}

// NTSTATUS LsaAddAccountRights(
//   [in] LSA_HANDLE          PolicyHandle,
//   [in] PSID                AccountSid,
//   [in] PLSA_UNICODE_STRING UserRights,
//   [in] ULONG               CountOfRights
// );
//
//sys lsaAddAccountRights(policy LSAHandle, account *windows.SID, userRights *LSAUnicodeString, userRightsCount uint32) (ntstatus error) = advapi32.LsaAddAccountRights

func LSAAddAccountRights(policy LSAHandle, account *windows.SID, userRights []LSAUnicodeString) error {
	if len(userRights) == 0 {
		return syscall.EINVAL
	}
	return lsaAddAccountRights(policy, account, &userRights[0], uint32(len(userRights)))
}
