//go:build windows

package util

import (
	"context"
	"path/filepath"
	"testing"
)

// windows only because TestingBinaryPath is Windows only as well, and the default path is `C:\`

// Default path to share the current testing binary under.
const ReExecDefaultGuestPathBase = `C:\`

// ReExecSelfGuestPath returns the path to the current testing binary, as well as the
// path should be shared (under the path specified by base).
//
// If base is empty, [ReExecDefaultGuestPathBase] will be used.
func ReExecSelfGuestPath(ctx context.Context, tb testing.TB, base string) (string, string) {
	tb.Helper()

	if base == "" {
		base = `C:\`
	}

	self := TestingBinaryPath(ctx, tb)
	return self, filepath.Join(base, filepath.Base(self))
}
