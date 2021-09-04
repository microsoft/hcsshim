package lcow

import (
	"bytes"
	"context"
	"fmt"
	"time"

	cmdpkg "github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/timeout"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/sirupsen/logrus"
)

// formatDiskUvm creates a utility vm, mounts the disk as a scsi disk onto to the VM
// and then formats it with ext4.
func formatDiskUvm(ctx context.Context, lcowUVM *uvm.UtilityVM, controller int, lun int32, destPath string) error {
	// Validate /sys/bus/scsi/devices/C:0:0:L exists as a directory
	devicePath := fmt.Sprintf("/sys/bus/scsi/devices/%d:0:0:%d/block", controller, lun)
	testdCtx, cancel := context.WithTimeout(ctx, timeout.TestDRetryLoop)
	defer cancel()
	for {
		cmd := cmdpkg.CommandContext(testdCtx, lcowUVM, "test", "-d", devicePath)
		err := cmd.Run()
		if err == nil {
			break
		}
		if _, ok := err.(*cmdpkg.ExitError); !ok {
			return fmt.Errorf("failed to run %+v following hot-add %s to utility VM: %s", cmd.Spec.Args, destPath, err)
		}
		time.Sleep(time.Millisecond * 10)
	}
	cancel()

	// Get the device from under the block subdirectory by doing a simple ls. This will come back as (eg) `sda`
	lsCtx, cancel := context.WithTimeout(ctx, timeout.ExternalCommandToStart)
	cmd := cmdpkg.CommandContext(lsCtx, lcowUVM, "ls", devicePath)
	lsOutput, err := cmd.Output()
	cancel()
	if err != nil {
		return fmt.Errorf("failed to `%+v` following hot-add %s to utility VM: %s", cmd.Spec.Args, destPath, err)
	}
	device := fmt.Sprintf(`/dev/%s`, bytes.TrimSpace(lsOutput))
	log.G(ctx).WithFields(logrus.Fields{
		"dest":   destPath,
		"device": device,
	}).Debug("lcow::FormatDisk device guest location")

	// Format it ext4
	mkfsCtx, cancel := context.WithTimeout(ctx, timeout.ExternalCommandToStart)
	cmd = cmdpkg.CommandContext(mkfsCtx, lcowUVM, "mkfs.ext4", "-q", "-E", "lazy_itable_init=0,nodiscard", "-O", `^has_journal,sparse_super2,^resize_inode`, device)
	var mkfsStderr bytes.Buffer
	cmd.Stderr = &mkfsStderr
	err = cmd.Run()
	cancel()
	if err != nil {
		return fmt.Errorf("failed to `%+v` following hot-add %s to utility VM: %s. detailed error: %s", cmd.Spec.Args, destPath, err, mkfsStderr.String())
	}

	log.G(ctx).WithField("dest", destPath).Debug("lcow::FormatDisk complete")

	return nil
}
