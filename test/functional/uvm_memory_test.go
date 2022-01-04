package functional

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
)

func TestUVMMemoryUpdateLCOW(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	opts := getDefaultLcowUvmOptions(t, t.Name())
	opts.MemorySizeInMB = 1024 * 2
	u := testutilities.CreateLCOWUVMFromOpts(ctx, t, nil, opts)
	defer u.Close()

	newMemorySize := uint64(opts.MemorySizeInMB/2) * bytesPerMB

	if err := u.UpdateMemory(ctx, newMemorySize); err != nil {
		t.Fatalf("failed to make call to modify UVM memory size in MB with: %v", err)
	}
	memInBytes, err := u.GetAssignedMemoryInBytes(ctx)
	if err != nil {
		t.Fatalf("failed to verified assigned UVM memory size")
	}
	if memInBytes != newMemorySize {
		t.Fatalf("incorrect memory size returned, expected %d but got %d", newMemorySize, memInBytes)
	}
}

func TestUVMMemoryUpdateWCOW(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)

	client, ctx := getCtrdClient(context.Background(), t)
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()

	opts := getDefaultWcowUvmOptions(t, t.Name())
	opts.MemorySizeInMB = 1024 * 2

	u, _, uvmScratchDir := testutilities.CreateWCOWUVMFromOptsWithImage(ctx, t, client, opts, "mcr.microsoft.com/windows/nanoserver:1909")
	defer os.RemoveAll(uvmScratchDir)
	defer u.Close()

	newMemoryInBytes := uint64(opts.MemorySizeInMB/2) * bytesPerMB
	if err := u.UpdateMemory(ctx, newMemoryInBytes); err != nil {
		t.Fatalf("failed to make call to modify UVM memory size in MB with: %v", err)
	}
	memInBytes, err := u.GetAssignedMemoryInBytes(ctx)
	if err != nil {
		t.Fatalf("failed to verified assigned UVM memory size")
	}
	if memInBytes != newMemoryInBytes {
		t.Fatalf("incorrect memory size returned, expected %d but got %d", newMemoryInBytes, memInBytes)
	}
}
