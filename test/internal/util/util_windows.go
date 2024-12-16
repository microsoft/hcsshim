//go:build windows

package util

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/Microsoft/go-winio/pkg/fs"

	"github.com/Microsoft/hcsshim/internal/wclayer"
)

// DestroyLayer is similar to [RemoveAll], but uses [wclayer.DestroyLayer] instead of [os.RemoveAll].
func DestroyLayer(ctx context.Context, p string) (err error) {
	// check if the path exists
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil
	}
	return repeat(func() error { return wclayer.DestroyLayer(ctx, p) }, RemoveAttempts, RemoveWait)
}

// executablePathOnce returns the current testing binary image
var executablePathOnce = sync.OnceValues(func() (string, error) {
	// use [os.Executable] over `os.Args[0]` to make sure path is absolute
	// as always, this shouldn't really fail, but just to be safe...
	p, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("retrieve executable path: %w", err)
	}
	// just to be safe (and address the comments on [os.Executable]), resolve the path
	return fs.ResolvePath(p)
})

func TestingBinaryPath(_ context.Context, tb testing.TB) string {
	tb.Helper()

	p, err := executablePathOnce()
	if err != nil {
		tb.Fatalf("could not get testing binary path: %v", err)
	}
	return p
}
