//go:build windows

package layers

import (
	"context"
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

func CacheFile(_ context.Context, tb testing.TB, dir string) string {
	tb.Helper()
	if dir == "" {
		dir = tb.TempDir()
		tb.Cleanup(func() {
			_ = util.RemoveAll(dir)
		})
	}
	cache := filepath.Join(dir, CacheFileName)
	return cache
}

// ScratchSpace creates an LCOW scratch space VHD at `dir\name`, and returns the dir and name.
// If name, (dir, or cache) are empty, ScratchSpace uses [ScratchSpace] (creates a temporary
// directory), respectively.
func ScratchSpace(ctx context.Context, tb testing.TB, vm *uvm.UtilityVM, name, dir, cache string) (string, string) {
	tb.Helper()
	if dir == "" {
		dir = tb.TempDir()
		tb.Cleanup(func() {
			_ = util.RemoveAll(dir)
		})
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
