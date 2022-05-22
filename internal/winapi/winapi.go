package winapi

//go:generate go run github.com/Microsoft/go-winio/tools/mkwinsyscall -sort=false -output zsyscall_windows.go bindflt.go user.go console.go system.go net.go path.go thread.go jobobject.go logon.go memory.go process.go processor.go devices.go filesystem.go errors.go
