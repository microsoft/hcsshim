//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLimitedRead verifies that limitedRead enforces the byte limit when the
// file is larger than the limit and reads the full content when the file is
// smaller than the limit.
func TestLimitedRead(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "panic.log")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	buf, err := limitedRead(filePath, 2)
	if err != nil {
		t.Fatalf("limitedRead: %v", err)
	}
	if string(buf) != "he" {
		t.Fatalf("expected 'he', got %q", string(buf))
	}

	buf, err = limitedRead(filePath, 10)
	if err != nil {
		t.Fatalf("limitedRead: %v", err)
	}
	if string(buf) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(buf))
	}
}

// TestLimitedReadMissingFile verifies that limitedRead returns an error when
// the target file does not exist.
func TestLimitedReadMissingFile(t *testing.T) {
	_, err := limitedRead(filepath.Join(t.TempDir(), "missing.log"), 10)
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
}
