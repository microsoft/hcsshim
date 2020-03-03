// +build functional

package cri_containerd

//go:generate go run ../../mksyscall_windows.go -output zsyscall_windows.go syscall.go

//sys hcsFormatWritableLayerVhd(handle uintptr) (hr error) = computestorage.HcsFormatWritableLayerVhd
