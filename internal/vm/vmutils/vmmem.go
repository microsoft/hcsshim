//go:build windows

package vmutils

import (
	"context"
	"errors"
	"iter"
	"strings"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/log"
	iwin "github.com/Microsoft/hcsshim/internal/windows"

	"github.com/Microsoft/go-winio/pkg/guid"
	"golang.org/x/sys/windows"
)

const (
	// vmmemProcessName is the name of the Hyper-V memory management process.
	vmmemProcessName = "vmmem"
	// vmmemProcessNameExt is the name of the process with .exe extension.
	vmmemProcessNameExt = "vmmem.exe"
	// ntVirtualMachineDomain is the domain name for Hyper-V virtual machine security principals.
	ntVirtualMachineDomain = "NT VIRTUAL MACHINE"
)

// allProcessEntries returns an iterator over all process entries in a Toolhelp32 snapshot.
// If the snapshot cannot be created or a process entry cannot be read, the error is logged and
// iteration stops.
func allProcessEntries(ctx context.Context, win iwin.API) iter.Seq[*windows.ProcessEntry32] {
	return func(yield func(*windows.ProcessEntry32) bool) {
		snapshot, err := win.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
		if err != nil {
			log.G(ctx).WithError(err).Error("failed to create process snapshot")
			return
		}
		defer func(win iwin.API, h windows.Handle) {
			_ = win.CloseHandle(h)
		}(win, snapshot)

		var pe32 windows.ProcessEntry32
		pe32.Size = uint32(unsafe.Sizeof(pe32))

		for err = win.Process32First(snapshot, &pe32); ; err = win.Process32Next(snapshot, &pe32) {
			if err != nil {
				log.G(ctx).WithError(err).Debug("finished iterating process entries")
				return
			}
			if !yield(&pe32) {
				return
			}
		}
	}
}

// LookupVMMEM locates the vmmem process for a VM given the VM ID.
// It enumerates processes using Toolhelp32 to filter by name, then validates
// the token using LookupAccount to match the "NT VIRTUAL MACHINE\<VM ID>" identity.
func LookupVMMEM(ctx context.Context, vmID guid.GUID, win iwin.API) (windows.Handle, error) {
	vmIDStr := strings.ToUpper(vmID.String())
	log.G(ctx).WithField("vmID", vmIDStr).Debug("looking up vmmem via LookupAccount")

	for pe32 := range allProcessEntries(ctx, win) {
		exeName := windows.UTF16ToString(pe32.ExeFile[:])

		// 1. Only target processes named vmmem or vmmem.exe.
		if !strings.EqualFold(exeName, vmmemProcessName) && !strings.EqualFold(exeName, vmmemProcessNameExt) {
			continue
		}

		// 2. Open the process to inspect its security token.
		pHandle, err := win.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe32.ProcessID)
		if err != nil {
			continue
		}

		var t windows.Token
		if err := win.OpenProcessToken(pHandle, windows.TOKEN_QUERY, &t); err != nil {
			_ = win.CloseHandle(pHandle)
			continue
		}

		tUser, err := win.GetTokenUser(t)
		if err != nil {
			_ = win.CloseToken(t)
			_ = win.CloseHandle(pHandle)
			continue
		}

		// 3. Use the OS API to resolve the SID to account and domain strings.
		account, domain, _, err := win.LookupAccount(tUser.User.Sid, "")
		_ = win.CloseToken(t)
		if err != nil {
			_ = win.CloseHandle(pHandle)
			continue
		}

		// 4. Compare against the expected Hyper-V UVM identity.
		if strings.EqualFold(domain, ntVirtualMachineDomain) && strings.EqualFold(account, vmIDStr) {
			log.G(ctx).WithField("pid", pe32.ProcessID).Debug("found vmmem match")
			return pHandle, nil
		}

		_ = win.CloseHandle(pHandle)
	}

	return 0, errors.New("failed to find matching vmmem process")
}
