//go:build windows

package vmutils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultLCOWOSBootFilesPath(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) (cleanup func())
		expectedCheck func(t *testing.T, result string)
	}{
		{
			name: "returns LinuxBootFiles subdirectory when it exists",
			setup: func(t *testing.T) func() {
				t.Helper()
				// Create a temporary LinuxBootFiles directory next to the test executable
				execDir := filepath.Dir(os.Args[0])
				linuxBootFilesPath := filepath.Join(execDir, "LinuxBootFiles")

				if err := os.MkdirAll(linuxBootFilesPath, 0755); err != nil {
					t.Fatalf("Failed to create test directory: %v", err)
				}

				return func() {
					_ = os.RemoveAll(linuxBootFilesPath)
				}
			},
			expectedCheck: func(t *testing.T, result string) {
				t.Helper()
				execDir := filepath.Dir(os.Args[0])
				expected := filepath.Join(execDir, "LinuxBootFiles")
				if result != expected {
					t.Errorf("DefaultLCOWOSBootFilesPath() = %q, expected %q", result, expected)
				}
			},
		},
		{
			name: "returns ProgramFiles Linux Containers when LinuxBootFiles does not exist",
			setup: func(t *testing.T) func() {
				t.Helper()
				// Make sure LinuxBootFiles does not exist
				execDir := filepath.Dir(os.Args[0])
				linuxBootFilesPath := filepath.Join(execDir, "LinuxBootFiles")
				// Attempt to remove in case it exists
				_ = os.RemoveAll(linuxBootFilesPath)
				return func() {}
			},
			expectedCheck: func(t *testing.T, result string) {
				t.Helper()
				expected := filepath.Join(os.Getenv("ProgramFiles"), "Linux Containers")
				if result != expected {
					t.Errorf("DefaultLCOWOSBootFilesPath() = %q, expected %q", result, expected)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleanup := tc.setup(t)
			defer cleanup()

			result := DefaultLCOWOSBootFilesPath()
			tc.expectedCheck(t, result)
		})
	}
}

func TestDefaultLCOWOSBootFilesPath_PathConstruction(t *testing.T) {
	// This test verifies the path construction logic without relying on file system state
	result := DefaultLCOWOSBootFilesPath()

	// The result should always be an absolute path
	if !filepath.IsAbs(result) {
		t.Errorf("DefaultLCOWOSBootFilesPath() returned non-absolute path: %q", result)
	}

	// The result should end with either "LinuxBootFiles" or "Linux Containers"
	base := filepath.Base(result)
	if base != "LinuxBootFiles" && base != "Linux Containers" {
		t.Errorf("DefaultLCOWOSBootFilesPath() returned unexpected base path: %q", base)
	}
}
