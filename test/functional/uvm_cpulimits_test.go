package functional

import (
	"context"
	"testing"
	"time"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/test/testutil"
)

func TestUVMCPULimitsUpdateLCOW(t *testing.T) {
	testutil.RequiresBuild(t, osversion.RS5)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	opts := getDefaultLCOWUvmOptions(t, t.Name())
	opts.MemorySizeInMB = 1024 * 2
	u := testutil.CreateLCOWUVMFromOpts(ctx, t, nil, opts)

	limits := &hcsschema.ProcessorLimits{
		Weight: 10000,
	}
	if err := u.UpdateCPULimits(ctx, limits); err != nil {
		t.Fatalf("failed to update the cpu limits of the UVM with: %v", err)
	}
}

func TestUVMCPULimitsUpdateWCOW(t *testing.T) {
	testutil.RequiresBuild(t, osversion.RS5)

	client, ctx := newCtrdClient(context.Background(), t)
	opts := getDefaultWCOWUvmOptions(t, t.Name())
	opts.MemorySizeInMB = 1024 * 2

	// context with time out times out and prevents this from cleaning up and removing snapshots
	u, _, _ := testutil.CreateWCOWUVMFromOptsWithImage(ctx, t, client, opts, testutil.ImageWindowsNanoserver2004)

	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	limits := &hcsschema.ProcessorLimits{
		Weight: 10000,
	}
	if err := u.UpdateCPULimits(ctx, limits); err != nil {
		t.Fatalf("failed to update the cpu limits of the UVM with: %v", err)
	}
}
