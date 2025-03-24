//go:build windows

package layers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/pkg/go-runhcs"

	"github.com/Microsoft/hcsshim/test/internal/util"
)

const (
	CacheFileName    = "cache.vhdx"
	ScratchSpaceName = "sandbox.vhdx"
)

func LCOWScratchCacheFile(ctx context.Context, tb testing.TB, dir string) string {
	tb.Helper()
	if dir == "" {
		dir = newTestTempDir(ctx, tb, "")
	}
	cache := filepath.Join(dir, CacheFileName)
	return cache
}

// GlobalLCOWScratchCacheFile returns a cache file that can be shared by all tests.
func GlobalLCOWScratchCacheFile(ctx context.Context, tb testing.TB) string {
	tb.Helper()

	return filepath.Join(getTempDir(ctx, tb), CacheFileName)
}

// ScratchSpace creates an LCOW scratch space VHD at `dir/name`, and returns the directory and
// scratch space file path.
// If name (or dir or cache) is empty, ScratchSpace uses [ScratchSpaceName] (creates a temporary
// directory), respectively.
func ScratchSpace(ctx context.Context, tb testing.TB, vm *uvm.UtilityVM, name, dir, cache string) (string, string) {
	tb.Helper()
	if dir == "" {
		dir = newTestTempDir(ctx, tb, vm.ID())
	}

	if cache == "" {
		cache = GlobalLCOWScratchCacheFile(ctx, tb)
	}
	if name == "" {
		name = ScratchSpaceName
	}
	scratch := filepath.Join(dir, name)

	tb.Logf("cache: %s", cache)
	tb.Logf("scratch: %s", scratch)

	// use `runhcs.exe` instead of calling ["github.com/Microsoft/hcsshim/internal/lcow".CreateScratch] directly to:
	//  1. use same code paths as containerd task creation
	//  2. avoid doing formatting operations in the uVM under test

	rhcs := runhcs.Runhcs{
		Debug:     true,
		Log:       filepath.Join(dir, "runhcs-scratch.log"),
		LogFormat: runhcs.JSON,
		Owner:     vm.Owner(),
	}

	opt := runhcs.CreateScratchOpts{
		CacheFile: cache,
	}

	// try to get the bootfiles path from the vm
	bootfiles, err := vm.LCOWBootFiles()
	if err != nil {
		tb.Fatalf("could not get LCOW uVM boot files path: %v", err)
	}
	opt.BootFiles = bootfiles

	if err := rhcs.CreateScratchWithOpts(ctx, scratch, &opt); err != nil {
		tb.Fatalf("could not create scratch space %q for vm %q using cache file %q: %v", scratch, vm.ID(), cache, err)
	}

	return dir, scratch
}

func WCOWScratchDir(ctx context.Context, tb testing.TB, dir string) string {
	tb.Helper()
	if dir == "" {
		dir = newTestTempDir(ctx, tb, "")
	}

	tb.Cleanup(func() {
		if err := util.DestroyLayer(ctx, dir); err != nil {
			tb.Errorf("failed to destroy %q: %v", dir, err)
		}
	})

	return dir
}

func newTestTempDir(ctx context.Context, tb testing.TB, name string) string {
	tb.Helper()
	dir := getTempDir(ctx, tb)

	if name == "" {
		name = util.CleanName(tb.Name())
	}
	dir, err := os.MkdirTemp(dir, name)
	if err != nil {
		tb.Fatalf("create test temp directory: %v", err)
	}

	tb.Cleanup(func() {
		_ = util.RemoveAll(dir)
	})

	return dir
}
