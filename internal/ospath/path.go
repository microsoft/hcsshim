package ospath

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var (
	errUnsafePath = errors.New("unsafe path detected")

	// DisallowedUVMMountPrefixes represents common locations within UVM
	// where we do not want mounts to happen from customer perspective.
	DisallowedUVMMountPrefixes = []string{
		`C:\Windows`,
		`C:\mounts`,
		`C:\EFI`,
		`C:\SandboxMounts`,
	}
)

// Join joins paths using the target OS's path separator.
func Join(os string, elem ...string) string {
	if os == "windows" {
		return filepath.Join(elem...)
	}
	return path.Join(elem...)
}

// Sanitize validates and normalizes a Windows path.
func Sanitize(path string, disallowedPrefixes []string) (string, error) {
	if path == "" {
		return "", errUnsafePath
	}

	// Normalize the path.
	cleanPath := filepath.Clean(path)

	// Reject UNC paths (\\server\share or //server/share)
	if strings.HasPrefix(cleanPath, `\\`) || strings.HasPrefix(cleanPath, `//`) {
		return "", errUnsafePath
	}

	// Check if the path is not in the disallowed paths.
	for _, prefix := range disallowedPrefixes {
		if strings.HasPrefix(cleanPath, prefix) {
			return "", errUnsafePath
		}
	}

	// Reject if the path already exists (file/dir/symlink/junction).
	// Use Lstat so we do not follow symlinks.
	var err error
	if _, err = os.Lstat(cleanPath); err == nil {
		// Path exists
		return "", fmt.Errorf("%w: path %q already exists", errUnsafePath, cleanPath)
	}
	if !os.IsNotExist(err) {
		// Unexpected error (e.g., permission issues)
		return "", fmt.Errorf("%w: error checking existence for %q: %w", errUnsafePath, cleanPath, err)
	}

	return cleanPath, nil
}
