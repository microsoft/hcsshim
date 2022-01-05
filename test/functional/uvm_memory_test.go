package functional

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/test/testutil"
)

func TestUVMMemoryUpdateLCOW(t *testing.T) {
	testutil.RequiresBuild(t, osversion.RS5)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	opts := getDefaultLCOWUvmOptions(t, t.Name())
	opts.MemorySizeInMB = 1024 * 2
	u := testutil.CreateLCOWUVMFromOpts(ctx, t, nil, opts)
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
	testutil.RequiresBuild(t, osversion.RS5)

	client, ctx := newCtrdClient(context.Background(), t)
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	opts := getDefaultWCOWUvmOptions(t, t.Name())
	opts.MemorySizeInMB = 1024 * 2

	u, _, uvmScratchDir := testutil.CreateWCOWUVMFromOptsWithImage(ctx, t, client, opts, testutil.ImageWindowsNanoserver2004)
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
