//go:build windows

package cimwriter

import (
	"os"
	"regexp"
	"testing"
)

func TestGetFileVersion_Kernel32(t *testing.T) {
	// kernel32.dll is always present and always has version info.
	path := os.Getenv("SystemRoot") + `\System32\kernel32.dll`
	ver, err := getFileVersion(path)
	if err != nil {
		t.Fatalf("getFileVersion(%q) failed: %v", path, err)
	}
	// Version should match "major.minor.build.revision" pattern.
	matched, _ := regexp.MatchString(`^\d+\.\d+\.\d+\.\d+$`, ver)
	if !matched {
		t.Fatalf("unexpected version format: %q", ver)
	}
	t.Logf("kernel32.dll version: %s", ver)
}

func TestGetFileVersion_NonexistentFile(t *testing.T) {
	_, err := getFileVersion(`C:\nonexistent_path\fake.dll`)
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestGetFileVersion_CimwriterDLL(t *testing.T) {
	if !Supported() {
		t.Skip("cimwriter.dll is not supported on this system")
	}
	path := os.Getenv("SystemRoot") + `\System32\cimwriter.dll`
	ver, err := getFileVersion(path)
	if err != nil {
		t.Fatalf("getFileVersion(%q) failed: %v", path, err)
	}
	matched, _ := regexp.MatchString(`^\d+\.\d+\.\d+\.\d+$`, ver)
	if !matched {
		t.Fatalf("unexpected version format: %q", ver)
	}
	t.Logf("cimwriter.dll version: %s", ver)
}
