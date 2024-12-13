//go:build linux
// +build linux

package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

// export this variable so it can be mocked to aid in testing for consuming packages
var filepathglob = filepath.Glob

// WaitForFileMatchingPattern waits for a single file that matches the given path pattern and returns the full path
// to the resulting file
func WaitForFileMatchingPattern(ctx context.Context, pattern string) (string, error) {
	for {
		files, err := filepathglob(pattern)
		if err != nil {
			return "", err
		}
		if len(files) == 0 {
			select {
			case <-ctx.Done():
				if err := ctx.Err(); err != nil {
					return "", fmt.Errorf("timed out waiting for file matching pattern %s to exist: %w", pattern, err)
				}
				return "", nil
			default:
				time.Sleep(time.Millisecond * 10)
				continue
			}
		} else if len(files) > 1 {
			return "", fmt.Errorf("more than one file could exist for pattern %q", pattern)
		}
		return files[0], nil
	}
}
