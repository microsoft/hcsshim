package ospath

import (
	"errors"
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

	// Require an absolute Windows path (drive letter + separator) so relative paths like
	// `..\..\Windows\System32` cannot bypass the absolute-prefix checks below.
	if len(cleanPath) < 3 || cleanPath[1] != ':' ||
		(cleanPath[2] != '\\' && cleanPath[2] != '/') ||
		((cleanPath[0] < 'a' || cleanPath[0] > 'z') &&
			(cleanPath[0] < 'A' || cleanPath[0] > 'Z')) {
		return "", errUnsafePath
	}

	// Reject if cleanPath equals or is under any disallowed prefix. Compare
	// case-insensitively (Windows) and enforce a path-separator boundary so
	// `C:\Windows` does not block `C:\WindowsBackup`.
	lowerPath := strings.ToLower(cleanPath)
	for _, prefix := range disallowedPrefixes {
		lowerPrefix := strings.ToLower(filepath.Clean(prefix))
		if lowerPath == lowerPrefix ||
			strings.HasPrefix(lowerPath, lowerPrefix+`\`) ||
			strings.HasPrefix(lowerPath, lowerPrefix+`/`) {
			return "", errUnsafePath
		}
	}

	return cleanPath, nil
}
