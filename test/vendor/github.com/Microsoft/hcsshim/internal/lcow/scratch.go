//go:build windows

package lcow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Microsoft/go-winio/vhd"
	cmdpkg "github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/timeout"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultScratchSizeGB is the size of the default LCOW scratch disk in GB
	DefaultScratchSizeGB = 20

	// defaultVhdxBlockSizeMB is the block-size for the scratch VHDx's this
	// package can create.
	defaultVhdxBlockSizeMB = 1
)

// CreateScratch uses a utility VM to create an empty scratch disk of a
// requested size. It has a caching capability. If the cacheFile exists, and the
// request is for a default size, a copy of that is made to the target. If the
// size is non-default, or the cache file does not exist, it uses a utility VM
// to create target. It is the responsibility of the caller to synchronize
// simultaneous attempts to create the cache file.
func CreateScratch(ctx context.Context, lcowUVM *uvm.UtilityVM, destFile string, sizeGB uint32, cacheFile string) error {
	if lcowUVM == nil {
		return fmt.Errorf("no uvm")
	}

	if lcowUVM.OS() != "linux" {
		return errors.New("lcow::CreateScratch requires a linux utility VM to operate")
	}

	log.G(ctx).WithFields(logrus.Fields{
		"dest":   destFile,
		"sizeGB": sizeGB,
		"cache":  cacheFile,
	}).Debug("lcow::CreateScratch opts")

	// Retrieve from cache if the default size and already on disk
	if cacheFile != "" && sizeGB == DefaultScratchSizeGB {
		if _, err := os.Stat(cacheFile); err == nil {
			if err := copyfile.CopyFile(ctx, cacheFile, destFile, false); err != nil {
				return fmt.Errorf("failed to copy cached file '%s' to '%s': %s", cacheFile, destFile, err)
			}
			log.G(ctx).WithFields(logrus.Fields{
				"dest":  destFile,
				"cache": cacheFile,
			}).Debug("lcow::CreateScratch copied from cache")
			return nil
		}
	}

	// Create the VHDX
	if err := vhd.CreateVhdx(destFile, sizeGB, defaultVhdxBlockSizeMB); err != nil {
		return fmt.Errorf("failed to create VHDx %s: %s", destFile, err)
	}

	var options []string
	scsi, err := lcowUVM.AddSCSI(
		ctx,
		destFile,
		"", // No destination as not formatted
		false,
		lcowUVM.ScratchEncryptionEnabled(),
		options,
		uvm.VMAccessTypeIndividual,
	)
	if err != nil {
		return err
	}
	removeSCSI := true
	defer func() {
		if removeSCSI {
			_ = lcowUVM.RemoveSCSI(ctx, destFile)
		}
	}()

	log.G(ctx).WithFields(logrus.Fields{
		"dest":       destFile,
		"controller": scsi.Controller,
		"lun":        scsi.LUN,
	}).Debug("lcow::CreateScratch device attached")

	// Validate /sys/bus/scsi/devices/C:0:0:L exists as a directory
	devicePath := fmt.Sprintf("/sys/bus/scsi/devices/%d:0:0:%d/block", scsi.Controller, scsi.LUN)
	testdCtx, cancel := context.WithTimeout(ctx, timeout.TestDRetryLoop)
	defer cancel()
	for {
		cmd := cmdpkg.CommandContext(testdCtx, lcowUVM, "test", "-d", devicePath)
		err := cmd.Run()
		if err == nil {
			break
		}
		if _, ok := err.(*cmdpkg.ExitError); !ok {
			return fmt.Errorf("failed to run %+v following hot-add %s to utility VM: %s", cmd.Spec.Args, destFile, err)
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
		return fmt.Errorf("failed to `%+v` following hot-add %s to utility VM: %s", cmd.Spec.Args, destFile, err)
	}
	device := fmt.Sprintf(`/dev/%s`, bytes.TrimSpace(lsOutput))
	log.G(ctx).WithFields(logrus.Fields{
		"dest":   destFile,
		"device": device,
	}).Debug("lcow::CreateScratch device guest location")

	// Format it ext4
	mkfsCtx, cancel := context.WithTimeout(ctx, timeout.ExternalCommandToStart)
	cmd = cmdpkg.CommandContext(mkfsCtx, lcowUVM, "mkfs.ext4", "-q", "-E", "lazy_itable_init=0,nodiscard", "-O", `^has_journal,sparse_super2,^resize_inode`, device)
	var mkfsStderr bytes.Buffer
	cmd.Stderr = &mkfsStderr
	err = cmd.Run()
	cancel()
	if err != nil {
		return fmt.Errorf("failed to `%+v` following hot-add %s to utility VM: %s", cmd.Spec.Args, destFile, err)
	}

	// Hot-Remove before we copy it
	removeSCSI = false
	if err := lcowUVM.RemoveSCSI(ctx, destFile); err != nil {
		return fmt.Errorf("failed to hot-remove: %s", err)
	}

	// Populate the cache.
	if cacheFile != "" && (sizeGB == DefaultScratchSizeGB) {
		if err := copyfile.CopyFile(ctx, destFile, cacheFile, true); err != nil {
			return fmt.Errorf("failed to seed cache '%s' from '%s': %s", destFile, cacheFile, err)
		}
	}

	log.G(ctx).WithField("dest", destFile).Debug("lcow::CreateScratch created (non-cache)")
	return nil
}
