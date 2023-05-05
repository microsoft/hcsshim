//go:build linux
// +build linux

package ext4

import (
	"context"
	"fmt"
	"os/exec"
)

// mkfsExt4Command runs mkfs.ext4 with the provided arguments
func mkfsExt4Command(args []string) error {
	cmd := exec.Command("mkfs.ext4", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute mkfs.ext4: %s: %w", string(output), err)
	}
	return nil
}
func Format(ctx context.Context, source string) error {
	// Format source as ext4
	if err := mkfsExt4Command([]string{source}); err != nil {
		return fmt.Errorf("mkfs.ext4 failed to format %s: %w", source, err)
	}
	return nil
}
