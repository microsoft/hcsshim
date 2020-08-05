package winapi

// This package contains bindings and constants from the win32 API that are
// needed for hcsshim. The main use is any bindings that aren't in golang.org/x/sys/windows

//go:generate go run ..\..\mksyscall_windows.go -output zsyscall_windows.go jobobject.go path.go logon.go memory.go processor.go
