//go:build windows

package util

import (
	"context"
	"os"

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
