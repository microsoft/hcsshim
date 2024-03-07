//go:build windows && functional

package functional

import (
	"context"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"

	"github.com/Microsoft/hcsshim/test/pkg/require"
	tuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func TestUVMMemoryUpdateLCOW(t *testing.T) {
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOW, featureUVM)

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	opts := defaultLCOWOptions(ctx, t)
	opts.MemorySizeInMB = 1024 * 2
	u := tuvm.CreateAndStartLCOWFromOpts(ctx, t, opts)
	defer u.Close()

	newMemorySize := uint64(opts.MemorySizeInMB/2) * memory.MiB

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
	t.Skip("not yet updated")

	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW, featureUVM)

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	opts := uvm.NewDefaultOptionsWCOW(t.Name(), "")
	opts.MemorySizeInMB = 1024 * 2

	//nolint:staticcheck // SA1019: deprecated; will be replaced when test is updated
	u, _, _ := tuvm.CreateWCOWUVMFromOptsWithImage(ctx, t, opts, "mcr.microsoft.com/windows/nanoserver:1909")
	defer u.Close()

	newMemoryInBytes := uint64(opts.MemorySizeInMB/2) * memory.MiB
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
