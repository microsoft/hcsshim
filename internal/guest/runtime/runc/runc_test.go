//go:build linux
// +build linux

package runc

import (
	"path/filepath"
	"testing"
)

func TestGetLogPath(t *testing.T) {
	got := getLogPath("/run/gcs/c/sandbox-1")
	want := "/run/gcs/c/sandbox-1/runc.log"
	if got != want {
		t.Errorf("getLogPath = %q, want %q", got, want)
	}
}

func TestGetContainerDir(t *testing.T) {
	r := &runcRuntime{}
	got := r.getContainerDir("test-id")
	want := filepath.Join(containerFilesDir, "test-id")
	if got != want {
		t.Errorf("getContainerDir = %q, want %q", got, want)
	}
}

func TestContainerBundlePath(t *testing.T) {
	c := &container{bundlePath: "/run/gcs/c/sb/ctr"}
	got := getLogPath(c.bundlePath)
	want := "/run/gcs/c/sb/ctr/runc.log"
	if got != want {
		t.Errorf("log path from bundlePath = %q, want %q", got, want)
	}
}
