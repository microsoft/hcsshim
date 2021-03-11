package main

import (
	"context"
	"log"
	"os"
	"syscall"

	"github.com/Microsoft/go-winio/vhd"
	"github.com/Microsoft/hcsshim/computestorage"
	"golang.org/x/sys/windows"
)

// Simple binary to create a vhd with a single NTFS partition.
func main() {
	if len(os.Args) < 2 {
		log.Fatal("must provide VHDX name")
	}

	vhdPath := os.Args[1]
	if err := vhd.CreateVhdx(vhdPath, 1, 1); err != nil {
		log.Fatalf("failed to create VHDX: %s", err)
	}

	vhdHandle, err := vhd.OpenVirtualDisk(vhdPath, vhd.VirtualDiskAccessNone, vhd.OpenVirtualDiskFlagNone)
	if err != nil {
		log.Fatalf("failed to open VHDX: %s", err)
	}
	defer syscall.CloseHandle(vhdHandle)

	if err := computestorage.FormatWritableLayerVhd(context.Background(), windows.Handle(vhdHandle)); err != nil {
		log.Fatalf("failed to format VHXD: %s", err)
	}

	os.Exit(0)
}
