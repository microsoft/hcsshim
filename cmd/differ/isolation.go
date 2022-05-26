//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/winapi"
)

func grantSIDsFileAccess(sids []*windows.SID, files []string, access windows.ACCESS_MASK) error {
fileLoop:
	for _, file := range files {
		for _, sid := range sids {
			if err := winapi.GrantSIDFileAccess(file, sid, access); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue fileLoop
				}
				return fmt.Errorf("grant sid %q access to file %q: %w", sid.String(), file, err)
			}
		}
	}
	return nil
}

func revokeSIDsFileAccess(sids []*windows.SID, files []string) error {
fileLoop:
	for _, file := range files {
		for _, sid := range sids {
			if err := winapi.RevokeSIDFileAccess(file, sid); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue fileLoop
				}
				return fmt.Errorf("grant sid %q access to file %q: %w", sid.String(), file, err)
			}
		}
	}
	return nil
}

// privilegesToDelete returns a list of all the privleges a token has, except for those
// specified in keep. The token must have been opened with the TOKEN_QUERY access.
func privilegesToDelete(token windows.Token, keep []string) ([]windows.LUIDAndAttributes, error) {
	keepLUIDs := make([]windows.LUID, 0, len(keep))
	for _, p := range keep {
		l, err := winapi.LookupPrivilegeValue(p)
		if err != nil {
			return nil, fmt.Errorf("lookup privilege to keep %q: %w", p, err)
		}
		keepLUIDs = append(keepLUIDs, l)
	}

	tpv, err := winapi.GetTokenPrivileges(token)
	if err != nil {
		return nil, fmt.Errorf("get token privileges: %w", err)
	}

	pvs := tpv.AllPrivileges()
	deletePvs := make([]windows.LUIDAndAttributes, 0, len(pvs))
	for _, a := range pvs {
		if shouldDeletePrivilege(&a, keepLUIDs) {
			deletePvs = append(deletePvs, a)
		}
	}

	return deletePvs, nil
}

func shouldDeletePrivilege(p *windows.LUIDAndAttributes, keep []windows.LUID) bool {
	for _, l := range keep {
		if p.Luid == l {
			return false
		}
	}
	return true
}
