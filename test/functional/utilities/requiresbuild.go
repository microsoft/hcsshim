package testutilities

import (
	"testing"

	"github.com/microsoft/hcsshim/osversion"
)

func RequiresBuild(t *testing.T, b uint16) {
	if osversion.Get().Build < b {
		t.Skipf("Requires build %d+", b)
	}
}

func RequiresExactBuild(t *testing.T, b uint16) {
	if osversion.Get().Build != b {
		t.Skipf("Requires exact build %d", b)
	}
}
