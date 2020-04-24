package kernel32

//go:generate go run ..\..\mksyscall_windows.go -output zsyscall_windows.go kernel32.go

//sys LocalAlloc(flags uint32, size int) (ptr uintptr) = kernel32.LocalAlloc
//sys LocalFree(ptr uintptr) = kernel32.LocalFree
