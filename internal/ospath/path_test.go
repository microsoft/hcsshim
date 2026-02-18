package ospath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	// Create a temp folder and file to simulate "already exists"
	existingDir := t.TempDir()
	existingFile := filepath.Join(existingDir, "exists.txt")
	if err := os.WriteFile(existingFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name               string
		path               string
		disallowedPrefixes []string
		expectedPath       string
		expectedErrPrefix  string
	}{
		{
			name:         "valid path",
			path:         filepath.Join(existingDir, "test"),
			expectedPath: filepath.Join(existingDir, "test"),
		},
		{
			name:              "empty path",
			path:              "",
			expectedErrPrefix: errUnsafePath.Error(),
		},
		{
			name:               "path traversal",
			path:               `C:\foo\..\Windows`,
			disallowedPrefixes: []string{`C:\Windows`},
			expectedErrPrefix:  errUnsafePath.Error(),
		},
		{
			name:              "UNC path",
			path:              `\\server\share`,
			expectedErrPrefix: errUnsafePath.Error(),
		},
		{
			name:               "disallowed prefix",
			path:               `C:\Windows\System32`,
			disallowedPrefixes: []string{`C:\Windows`},
			expectedErrPrefix:  errUnsafePath.Error(),
		},
		{
			name:              "existing folder",
			path:              existingDir,
			expectedErrPrefix: "already exists",
		},
		{
			name:              "existing file",
			path:              existingFile,
			expectedErrPrefix: "already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Sanitize(tt.path, tt.disallowedPrefixes)

			if tt.expectedErrPrefix != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.expectedErrPrefix)
				}
				if !strings.Contains(err.Error(), tt.expectedErrPrefix) {
					t.Fatalf("expected error to contain %q, got %v", tt.expectedErrPrefix, err)
				}
			} else if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if !strings.EqualFold(got, tt.expectedPath) {
				t.Errorf("expected path %q, got %q", tt.expectedPath, got)
			}
		})
	}
}
