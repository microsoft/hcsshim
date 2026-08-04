//go:build windows
// +build windows

package bridge

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStageDLL_Copies(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	contents := []byte("fake-dll-bytes")
	srcPath := filepath.Join(srcDir, amdSnpPspDLLName)
	if err := os.WriteFile(srcPath, contents, 0644); err != nil {
		t.Fatalf("failed to write source dll: %v", err)
	}

	staged, err := stageDLL(context.Background(), srcPath, dstDir)
	if err != nil {
		t.Fatalf("stageDLL returned error: %v", err)
	}
	if !staged {
		t.Fatal("expected staged to be true")
	}

	// The DLL should be copied into dstDir with identical contents.
	dstPath := filepath.Join(dstDir, amdSnpPspDLLName)
	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read staged dll: %v", err)
	}
	if !bytes.Equal(got, contents) {
		t.Errorf("staged dll contents = %q, want %q", got, contents)
	}
}

func TestStageDLL_MissingSourceIsNoOp(t *testing.T) {
	dstDir := t.TempDir()
	srcPath := filepath.Join(t.TempDir(), amdSnpPspDLLName) // does not exist

	staged, err := stageDLL(context.Background(), srcPath, dstDir)
	if err != nil {
		t.Fatalf("stageDLL returned error: %v", err)
	}
	if staged {
		t.Fatal("expected staged to be false when source is missing")
	}

	// No file should have been written to dstDir.
	if entries, err := os.ReadDir(dstDir); err != nil {
		t.Fatalf("failed to read dstDir: %v", err)
	} else if len(entries) != 0 {
		t.Errorf("expected dstDir to be empty, found %d entries", len(entries))
	}
}
