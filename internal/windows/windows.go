//go:build windows

package windows

import (
	"golang.org/x/sys/windows"
)

// WinAPI is the real implementation of windows.API that delegates to the actual Windows API.
type WinAPI struct{}

// CreateToolhelp32Snapshot takes a snapshot of the specified processes.
func (w *WinAPI) CreateToolhelp32Snapshot(flags uint32, processID uint32) (windows.Handle, error) {
	return windows.CreateToolhelp32Snapshot(flags, processID)
}

// CloseHandle closes an open object handle.
func (w *WinAPI) CloseHandle(h windows.Handle) error {
	return windows.CloseHandle(h)
}

// Process32First retrieves information about the first process in a snapshot.
func (w *WinAPI) Process32First(snapshot windows.Handle, pe *windows.ProcessEntry32) error {
	return windows.Process32First(snapshot, pe)
}

// Process32Next retrieves information about the next process in a snapshot.
func (w *WinAPI) Process32Next(snapshot windows.Handle, pe *windows.ProcessEntry32) error {
	return windows.Process32Next(snapshot, pe)
}

// OpenProcess opens an existing local process object.
func (w *WinAPI) OpenProcess(desiredAccess uint32, inheritHandle bool, processID uint32) (windows.Handle, error) {
	return windows.OpenProcess(desiredAccess, inheritHandle, processID)
}

// OpenProcessToken opens the access token associated with a process.
func (w *WinAPI) OpenProcessToken(process windows.Handle, desiredAccess uint32, token *windows.Token) error {
	return windows.OpenProcessToken(process, desiredAccess, token)
}

// GetTokenUser retrieves the user account of the token.
func (w *WinAPI) GetTokenUser(token windows.Token) (*windows.Tokenuser, error) {
	return token.GetTokenUser()
}

// LookupAccount retrieves the name of the account for a SID and the name of the first domain on which this SID is found.
func (w *WinAPI) LookupAccount(sid *windows.SID, system string) (account string, domain string, accType uint32, err error) {
	return sid.LookupAccount(system)
}

// CloseToken closes a token handle.
func (w *WinAPI) CloseToken(token windows.Token) error {
	return token.Close()
}
