//go:build windows

package require

import (
	"testing"

	"github.com/Microsoft/hcsshim/osversion"

	_ "github.com/Microsoft/hcsshim/test/internal/manifest" // manifest test binary automatically
)

func Build(t testing.TB, b uint16) {
	if osversion.Build() < b {
		t.Skipf("Requires build %d+", b)
	}
}

func ExactBuild(t testing.TB, b uint16) {
	if osversion.Build() != b {
		t.Skipf("Requires exact build %d", b)
	}
}
