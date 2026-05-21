package ospath

import (
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	tests := []struct {
		name               string
		path               string
		disallowedPrefixes []string
		expectedPath       string
		expectedErrPrefix  string
	}{
		{
			name:         "valid path",
			path:         `C:\custom\hpc`,
			expectedPath: `C:\custom\hpc`,
		},
		{
			name:              "empty path",
			path:              "",
			expectedErrPrefix: errUnsafePath.Error(),
		},
		{
			name:               "path traversal normalizes into disallowed",
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
			name:               "disallowed prefix - subpath",
			path:               `C:\Windows\System32`,
			disallowedPrefixes: []string{`C:\Windows`},
			expectedErrPrefix:  errUnsafePath.Error(),
		},
		{
			name:               "disallowed prefix - exact match",
			path:               `C:\Windows`,
			disallowedPrefixes: []string{`C:\Windows`},
			expectedErrPrefix:  errUnsafePath.Error(),
		},
		{
			name:               "disallowed prefix - case insensitive",
			path:               `C:\windows\System32`,
			disallowedPrefixes: []string{`C:\Windows`},
			expectedErrPrefix:  errUnsafePath.Error(),
		},
		{
			name:               "similarly-named sibling - allowed",
			path:               `C:\WindowsBackup`,
			disallowedPrefixes: []string{`C:\Windows`},
			expectedPath:       `C:\WindowsBackup`,
		},
		{
			name:               "no disallowed prefixes - allowed",
			path:               `C:\hpc`,
			disallowedPrefixes: nil,
			expectedPath:       `C:\hpc`,
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
