package lcow

import (
	"context"
	"errors"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/sirupsen/logrus"
)

// FormatDisk creates a utility vm, mounts the disk as a scsi disk onto to the VM
// and then formats it with ext4. Disk is expected to be made offline before this
// command is run. The following powershell commands:
// 'Get-Disk -Number <disk num> | Set-Disk -IsOffline $true'
// can be used to offline the disk.
func FormatDisk(ctx context.Context, lcowUVM *uvm.UtilityVM, destPath string) error {
	if lcowUVM == nil {
		return fmt.Errorf("no uvm")
	}

	if lcowUVM.OS() != "linux" {
		return errors.New("lcow::FormatDisk requires a linux utility VM to operate")
	}

	log.G(ctx).WithFields(logrus.Fields{
		"dest": destPath,
	}).Debug("lcow::FormatDisk opts")

	var options []string
	scsi, err := lcowUVM.AddSCSIPhysicalDisk(ctx, destPath, "", false, options) // No destination as not formatted
	if err != nil {
		return err
	}

	defer func() {
		_ = scsi.Release(ctx)
	}()

	log.G(ctx).WithFields(logrus.Fields{
		"dest":       destPath,
		"controller": scsi.Controller,
		"lun":        scsi.LUN,
	}).Debug("lcow::FormatDisk device attached")

	if err := formatDiskUvm(ctx, lcowUVM, scsi.Controller, scsi.LUN, destPath); err != nil {
		return err
	}
	log.G(ctx).WithField("dest", destPath).Debug("lcow::FormatDisk complete")

	return nil
}
