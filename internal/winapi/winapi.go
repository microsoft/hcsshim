/*Package winapi contains various low-level bindings to Windows APIs. It can
be thought of as an extension to golang.org/x/sys/windows. */
package winapi

//go:generate go run ..\..\mksyscall_windows.go -output zsyscall_windows.go devices.go heapalloc.go ntfs.go errors.go
