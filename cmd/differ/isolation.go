//go:build windows

package main

import (
	"fmt"

	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/winapi"
)

// restrictedToken creates a restricted token from the current process's token, deleting
// all privileges except those in keep.
func restrictedToken(keep []string) (t windows.Token, err error) {
	var etoken windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_DUPLICATE|windows.TOKEN_ASSIGN_PRIMARY|windows.TOKEN_QUERY|windows.TOKEN_WRITE,
		&etoken,
	); err != nil {
		return t, fmt.Errorf("open process token: %w", err)
	}
	defer etoken.Close()

	deleteLUIDs, err := privilegesToDelete(etoken, keep)
	if err != nil {
		return t, fmt.Errorf("get privileges to delete: %w", err)
	}

	if err := winapi.CreateRestrictedToken(
		etoken,
		0,   // flags
		nil, // SIDs to disable
		deleteLUIDs,
		nil, // SIDs to restrict
		&t,
	); err != nil {
		return t, fmt.Errorf("create restricted token: %w", err)
	}

	return t, nil
}

// privilegesToDelete returns a list of all the privleges a token has, except for those
// specified in keep.
//
// The return is a pointer to the first element of a []
func privilegesToDelete(token windows.Token, keep []string) ([]windows.LUIDAndAttributes, error) {
	keepLUIDs := make([]windows.LUID, 0, len(keep))
	for _, p := range keep {
		l, err := winapi.LookupPrivilegeValue(p)
		if err != nil {
			return nil, fmt.Errorf("lookup privilege to keep %q: %w", p, err)
		}
		keepLUIDs = append(keepLUIDs, l)
	}

	pv, err := winapi.GetTokenPrivileges(token)
	if err != nil {
		return nil, fmt.Errorf("get token privileges: %w", err)
	}

	privs := pv.AllPrivileges()
	privDel := make([]windows.LUIDAndAttributes, 0, len(privs))

	for _, a := range privs {
		if deletePriv(&a, keepLUIDs) {
			privDel = append(privDel, a)
		}
	}

	return privDel, nil
}

func deletePriv(p *windows.LUIDAndAttributes, keep []windows.LUID) bool {
	for _, l := range keep {
		if p.Luid == l {
			return false
		}
	}
	return true
}
