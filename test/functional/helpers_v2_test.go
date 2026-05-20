//go:build windows && functional

package functional

import "testing"

// isLCOWV2 reports whether the LCOWV2 feature flag is set on the current test
// invocation. Callers should prefer the higher-level helper requireV1Only so
// the V2 selection logic stays in one place.
func isLCOWV2() bool {
	return flagFeatures.IsSet(featureLCOWV2)
}

// requireV1Only skips the test when the LCOWV2 feature flag is set. Use this
// for tests that depend on V1-only features such as VPMEM, VHD/initrd boot
// modes, KernelDirect, or other UVM knobs not exposed in the v2 builder.
//
// Mirrors the pattern established in the azcri repo and in the CRI test suite
// in test/cri-containerd/.
func requireV1Only(tb testing.TB) {
	tb.Helper()
	if isLCOWV2() {
		tb.Skip("test requires V1 shim features (VPMEM/VHD/initrd/KernelDirect) not exposed in V2")
	}
}
