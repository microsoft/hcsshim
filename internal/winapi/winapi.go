package winapi

//go:generate go run golang.org/x/sys/windows/mkwinsyscall -output zsyscall_windows.go bindflt.go console.go devices.go errors.go filesystem.go jobobject.go logon.go memory.go net.go path.go privilege.go process.go processor.go sid.go system.go thread.go token.go user.go
