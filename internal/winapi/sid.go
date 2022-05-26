//go:build windows

package winapi

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	CapabilityLPACAppExperience                       = "lpacAppExperience"
	CapabilityLPACCryptoServices                      = "lpacCryptoServices"
	CapabilityLPACCom                                 = "lpacCom"
	CapabilityLPACIdentityServices                    = "lpacIdentityServices"
	CapabilityLPACEnterprisePolicyChangeNotifications = "lpacEnterprisePolicyChangeNotifications"
	CapabilityRegistryRead                            = "registryRead"
)

// GetAppAuthoritySIDFromName returns the capability with AppAuthority SID.
func GetAppAuthoritySIDFromName(capability string) (*windows.SID, error) {
	_, sids, err := DeriveCapabilitySIDsFromName(capability)
	if err != nil {
		return nil, err
	}
	if len(sids) == 0 {
		return nil, fmt.Errorf("derive capability did not return AppAuthority SIDs: %w", syscall.EINVAL)
	}
	return sids[0], nil
}

// GetGroupSIDFromName returns the group capability with NTAuthority SID.
func GetGroupSIDFromName(capability string) (*windows.SID, error) {
	_, sids, err := DeriveCapabilitySIDsFromName(capability)
	if err != nil {
		return nil, err
	}
	if len(sids) == 0 {
		return nil, fmt.Errorf("derive capability did not return AppAuthority SIDs: %w", syscall.EINVAL)
	}
	return sids[0], nil
}

// BOOL DeriveCapabilitySidsFromName(
//   [in]  LPCWSTR CapName,
//   [out] PSID    **CapabilityGroupSids,
//   [out] DWORD   *CapabilityGroupSidCount,
//   [out] PSID    **CapabilitySids,
//   [out] DWORD   *CapabilitySidCount
// );
//
//sys deriveCapabilitySIDsFromName(capability string, groupSIDs ***windows.SID, groupSIDsCount *uint32,  sids ***windows.SID, sidsCount *uint32) (err error) = kernelbase.DeriveCapabilitySidsFromName

// DeriveCapabilitySIDsFromName returns the group capability with NTAuthority SIDs and the capability
// with AppAuthority SIDs.
func DeriveCapabilitySIDsFromName(capability string) ([]*windows.SID, []*windows.SID, error) {
	var gsPtr, ssPtr **windows.SID
	var gsL, ssL uint32
	if err := deriveCapabilitySIDsFromName(capability, &gsPtr, &gsL, &ssPtr, &ssL); err != nil {
		return nil, nil, err
	}

	_gs := unsafe.Slice(gsPtr, gsL)
	gs, err := copyAndFreeSIDs(_gs)
	if err != nil {
		return nil, nil, err
	}
	_ss := unsafe.Slice(ssPtr, ssL)
	ss, err := copyAndFreeSIDs(_ss)
	if err != nil {
		return nil, nil, err
	}
	return gs, ss, nil
}

// copyAndFreeSIDs creates a copy of the SIDs array and frees the originalSIDs and the array allocated
// by the kernel via LocalFree,
func copyAndFreeSIDs(ss []*windows.SID) (ss2 []*windows.SID, err error) {
	if len(ss) == 0 {
		return nil, nil
	}
	defer LocalFree(uintptr(unsafe.Pointer(&ss[0])))

	ss2 = make([]*windows.SID, len(ss))
	for i, s := range ss {
		ss2[i], err = s.Copy()
		if err != nil {
			return nil, err
		}
		LocalFree(uintptr(unsafe.Pointer(s)))
	}
	return ss2, nil
}

// PVOID FreeSid(
//   [in] PSID pSid
// );
//
//sys FreeSID(s *windows.SID) (err error) [failretval!=0] = advapi32.FreeSid
