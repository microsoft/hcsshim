//go:build windows

package windows

import (
	"golang.org/x/sys/windows"
)

// API abstracts Windows API calls for testing purposes.
type API interface {
	// CreateToolhelp32Snapshot takes a snapshot of the specified processes.
	CreateToolhelp32Snapshot(flags uint32, processID uint32) (windows.Handle, error)

	// CloseHandle closes an open object handle.
	CloseHandle(h windows.Handle) error

	// Process32First retrieves information about the first process in a snapshot.
	Process32First(snapshot windows.Handle, pe *windows.ProcessEntry32) error

	// Process32Next retrieves information about the next process in a snapshot.
	Process32Next(snapshot windows.Handle, pe *windows.ProcessEntry32) error

	// OpenProcess opens an existing local process object.
	OpenProcess(desiredAccess uint32, inheritHandle bool, processID uint32) (windows.Handle, error)

	// OpenProcessToken opens the access token associated with a process.
	OpenProcessToken(process windows.Handle, desiredAccess uint32, token *windows.Token) error

	// GetTokenUser retrieves the user account of the token.
	GetTokenUser(token windows.Token) (*windows.Tokenuser, error)

	// LookupAccount retrieves the name of the account for a SID and the name of the first domain on which this SID is found.
	LookupAccount(sid *windows.SID, system string) (account string, domain string, accType uint32, err error)

	// CloseToken closes a token handle.
	CloseToken(token windows.Token) error
}
