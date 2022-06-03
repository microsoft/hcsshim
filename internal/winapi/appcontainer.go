//go:build windows

package winapi

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

//nolint:revive,stylecheck,deadcode
const (
	PROC_THREAD_ATTRIBUTE_ALL_APPLICATION_PACKAGES_POLICY = (15 & _PROC_THREAD_ATTRIBUTE_NUMBER) | _PROC_THREAD_ATTRIBUTE_INPUT

	PROCESS_CREATION_ALL_APPLICATION_PACKAGES_OPT_OUT = uint32(0x01)
)

// RequiredAppContainerEnvKeys are the needed environment variable keys for an app container.
// The value can be populated from the current environment.
func AppContainerRequiredEnvKeys() []string {
	return []string{
		"LOCALAPPDATA",
	}
}

// typedef struct _SECURITY_CAPABILITIES {
//     PSID AppContainerSid;
//     PSID_AND_ATTRIBUTES Capabilities;
//     ULONG CapabilityCount;
//     ULONG Reserved;
// } SECURITY_CAPABILITIES, *PSECURITY_CAPABILITIES, *LPSECURITY_CAPABILITIES;

type SecurityCapabilities struct {
	AppContainerSid *windows.SID
	Capabilities    *windows.SIDAndAttributes
	CapabilityCount uint32
	_               uint32 //Reserved
}

// USERENVAPI HRESULT CreateAppContainerProfile(
//   [in]  PCWSTR              pszAppContainerName,
//   [in]  PCWSTR              pszDisplayName,
//   [in]  PCWSTR              pszDescription,
//   [in]  PSID_AND_ATTRIBUTES pCapabilities,
//   [in]  DWORD               dwCapabilityCount,
//   [out] PSID                *ppSidAppContainerSid
// );
//
//sys createAppContainerProfile(appContainerName string, displayName string, description string, capabilities *windows.SIDAndAttributes, capabilitiesCount uint32, sidAppContainerSid **windows.SID) (hr error) = userenv.CreateAppContainerProfile

func CreateAppContainerProfile(name string, displayName string, description string, capabilities []windows.SIDAndAttributes) (*windows.SID, error) {
	sid := &windows.SID{}
	name = trimLen(name, 64)
	displayName = trimLen(displayName, 512)
	description = trimLen(description, 2048)
	// todo: use generic unslice func defined for token.go
	lcaps := uint32(len(capabilities))
	var pcaps *windows.SIDAndAttributes
	if lcaps > 0 {
		pcaps = &capabilities[0]
	}

	err := createAppContainerProfile(name, displayName, description, pcaps, lcaps, &sid)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if err = deriveAppContainerSidFromAppContainerName(name, &sid); err != nil {
			return nil, err
		}
	} else if err != nil {
		fmt.Println("could not create app container")
		return nil, err
	}

	defer func() {
		_ = FreeSID(sid)
	}()
	return sid.Copy()
}

func trimLen(s string, l int) string {
	if len(s) < l {
		l = len(s)
	}
	return s[:l]
}

// USERENVAPI HRESULT DeleteAppContainerProfile(
//   [in] PCWSTR pszAppContainerName
// );
//
//sys DeleteAppContainerProfile(appContainerName string) (hr error) = userenv.DeleteAppContainerProfile

// USERENVAPI HRESULT DeriveAppContainerSidFromAppContainerName(
//   [in]  PCWSTR pszAppContainerName,
//   [out] PSID   *ppsidAppContainerSid
// );
//
//sys deriveAppContainerSidFromAppContainerName(appContainerName string, appContainerSid **windows.SID) (hr error) = userenv.DeriveAppContainerSidFromAppContainerName

func DeriveAppContainerSidFromAppContainerName(name string) (*windows.SID, error) {
	sid := &windows.SID{}
	if err := deriveAppContainerSidFromAppContainerName(name, &sid); err != nil {
		return nil, err
	}
	defer func() {
		_ = FreeSID(sid)
	}()
	return sid, nil
}

func IsAppContainerToken(token windows.Token) (bool, error) {
	// todo:  If the current token is not an app container but is an identity level token, you should return AccessDenied.
	// (from https://docs.microsoft.com/en-us/windows/win32/api/winnt/ne-winnt-token_information_class#constants)
	var r uint64
	b := (*byte)(unsafe.Pointer(&r))
	l := uint32(unsafe.Sizeof(r))
	if err := windows.GetTokenInformation(token, TokenIsAppContainer, b, l, &l); err != nil {
		return false, fmt.Errorf("get token information: %w", err)
	}
	return r != 0, nil
}
