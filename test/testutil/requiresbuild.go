package testutil

import (
	"testing"

	"github.com/Microsoft/hcsshim/osversion"
)

func RequiresBuild(t *testing.T, b uint16) {
	if osversion.Build() < b {
		t.Skipf("Requires build %d+", b)
	}
}

func RequiresExactBuild(t *testing.T, b uint16) {
	if osversion.Build() != b {
		t.Skipf("Requires exact build %d", b)
	}
}
