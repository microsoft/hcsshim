package testutilities

import (
	"os"
	"testing"
)

// CreateTempDir creates a temporary directory
func CreateTempDir(t *testing.T) string {
	tempDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %s", err)
	}
	return tempDir
}
