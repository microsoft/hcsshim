package xfs

import (
	"fmt"
	"os/exec"
)

// Format formats `source` by invoking mkfs.xfs.
func Format(devicePath string) error {
	args := []string{"-f", devicePath}
	cmd := exec.Command("mkfs.xfs", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkfs.xfs failed with %s: %w", string(output), err)
	}
	return nil
}
