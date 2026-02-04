// Package winapi contains various low-level bindings to Windows APIs. It can
// be thought of as an extension to golang.org/x/sys/windows.
package winapi

//go:generate go tool github.com/Microsoft/go-winio/tools/mkwinsyscall -output zsyscall_windows.go ./*.go
