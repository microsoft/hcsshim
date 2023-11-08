//go:build windows

package layers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/internal/lcow"
	"github.com/Microsoft/hcsshim/internal/uvm"

	"github.com/Microsoft/hcsshim/test/internal/util"
)

const (
	CacheFileName       = "cache.vhdx"
	ScratchSpaceName    = "sandbox.vhdx"
	UVMScratchSpaceName = "uvmscratch.vhdx"
)

func CacheFile(ctx context.Context, tb testing.TB, dir string) string {
	tb.Helper()
	if dir == "" {
		dir = newTestTempDir(ctx, tb, "")
	}
	cache := filepath.Join(dir, CacheFileName)
	return cache
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
		cache = CacheFile(ctx, tb, dir)
	}
	if name == "" {
		name = ScratchSpaceName
	}
	scratch := filepath.Join(dir, name)

	if err := lcow.CreateScratch(ctx, vm, scratch, lcow.DefaultScratchSizeGB, cache); err != nil {
		tb.Fatalf("could not create scratch space %q using vm %q and cache file %q: %v", scratch, vm.ID(), cache, err)
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
	dir, err := tempDirOnce(ctx)
	if err != nil {
		tb.Fatal(err)
	}

	if name == "" {
		name = util.CleanName(tb.Name())
	}
	dir, err = os.MkdirTemp(dir, name)
	if err != nil {
		tb.Fatalf("create test temp directory: %v", err)
	}

	tb.Cleanup(func() {
		_ = util.RemoveAll(dir)
	})

	return dir
}
