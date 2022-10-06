//go:build !linux
// +build !linux

package compactext4

import "testing"

func verifyTestFile(t *testing.T, mountPath string, tf testFile) {
	t.Helper()
}

func mountImage(t *testing.T, image string, mountPath string) bool {
	t.Helper()
	return false
}

func unmountImage(t *testing.T, mountPath string) {
	t.Helper()
}

func fsck(t *testing.T, image string) {
	t.Helper()
}
