//go:build windows

package require

import (
	"testing"

	"github.com/Microsoft/hcsshim/osversion"

	_ "github.com/Microsoft/hcsshim/test/internal/manifest" // manifest test binary automatically
)

func Build(tb testing.TB, b uint16) {
	tb.Helper()
	if osversion.Build() < b {
		tb.Skipf("Requires build %d+", b)
	}
}

func ExactBuild(tb testing.TB, b uint16) {
	tb.Helper()
	if osversion.Build() != b {
		tb.Skipf("Requires exact build %d", b)
	}
}
