package functional

import (
	"context"
	"testing"
	"time"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
)

func TestUVMCPULimitsUpdateLCOW(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	opts := getDefaultLCOWUvmOptions(t, t.Name())
	opts.MemorySizeInMB = 1024 * 2
	u := testutilities.CreateLCOWUVMFromOpts(ctx, t, nil, opts)

	limits := &hcsschema.ProcessorLimits{
		Weight: 10000,
	}
	if err := u.UpdateCPULimits(ctx, limits); err != nil {
		t.Fatalf("failed to update the cpu limits of the UVM with: %v", err)
	}
}

func TestUVMCPULimitsUpdateWCOW(t *testing.T) {
	testutilities.RequiresBuild(t, osversion.RS5)

	client, ctx := newCtrdClient(context.Background(), t)
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()

	opts := getDefaultWCOWUvmOptions(t, t.Name())
	opts.MemorySizeInMB = 1024 * 2

	u, _, _ := testutilities.CreateWCOWUVMFromOptsWithImage(ctx, t, client, opts, testutilities.ImageWindowsNanoserver2004)

	limits := &hcsschema.ProcessorLimits{
		Weight: 10000,
	}
	if err := u.UpdateCPULimits(ctx, limits); err != nil {
		t.Fatalf("failed to update the cpu limits of the UVM with: %v", err)
	}
}
