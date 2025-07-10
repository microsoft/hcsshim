//go:build windows

package lcow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Microsoft/go-winio/vhd"
	"github.com/sirupsen/logrus"

	cmdpkg "github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/timeout"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
)

const (
	// DefaultScratchSizeGB is the size of the default LCOW scratch disk in GB.
	DefaultScratchSizeGB = 20

	// defaultVHDxBlockSizeMB is the block-size for the scratch VHDx's this
	// package can create.
	defaultVHDxBlockSizeMB = 1
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
				return fmt.Errorf("failed to copy cached file '%s' to '%s': %w", cacheFile, destFile, err)
			}
			log.G(ctx).WithFields(logrus.Fields{
				"dest":  destFile,
				"cache": cacheFile,
			}).Debug("lcow::CreateScratch copied from cache")
			return nil
		}
	}

	// Create the VHDX
	if err := vhd.CreateVhdx(destFile, sizeGB, defaultVHDxBlockSizeMB); err != nil {
		return fmt.Errorf("failed to create VHDx %s: %w", destFile, err)
	}

	// Attach VHDX as SCSI and mount as block device
	scsiMount, err := lcowUVM.SCSIManager.AddVirtualDisk(
		ctx,
		destFile,
		false,
		lcowUVM.ID(),
		"",
		&scsi.MountConfig{
			BlockDev: true,
		},
	)
	if err != nil {
		return err
	}
	removeSCSI := true
	defer func() {
		if removeSCSI {
			_ = scsiMount.Release(ctx)
		}
	}()

	log.G(ctx).WithFields(logrus.Fields{
		"dest":       destFile,
		"controller": scsiMount.Controller(),
		"lun":        scsiMount.LUN(),
		"blockdev":   scsiMount.GuestPath(),
	}).Debug("lcow::CreateScratch device attached")

	// Format block device mount as ext4
	mkfsCtx, cancel := context.WithTimeout(ctx, timeout.ExternalCommandToStart)
	cmd := cmdpkg.CommandContext(mkfsCtx, lcowUVM, "mkfs.ext4", "-q", "-E", "lazy_itable_init=0,nodiscard", "-O", `^has_journal,sparse_super2,^resize_inode`, scsiMount.GuestPath())
	var mkfsStderr bytes.Buffer
	cmd.Stderr = &mkfsStderr
	err = cmd.Run()
	cancel()
	if err != nil {
		log.G(ctx).WithError(err).WithField("stderr", mkfsStderr.String()).Error("mkfs.ext4 failed")
		return fmt.Errorf("failed to `%+v` following hot-add %s to utility VM: %w", cmd.Spec.Args, destFile, err)
	}

	// Hot-Remove before we copy it
	removeSCSI = false
	if err := scsiMount.Release(ctx); err != nil {
		return fmt.Errorf("failed to hot-remove: %w", err)
	}

	// Populate the cache.
	if cacheFile != "" && (sizeGB == DefaultScratchSizeGB) {
		if err := copyfile.CopyFile(ctx, destFile, cacheFile, true); err != nil {
			return fmt.Errorf("failed to seed cache '%s' from '%s': %w", destFile, cacheFile, err)
		}
	}

	log.G(ctx).WithField("dest", destFile).Debug("lcow::CreateScratch created (non-cache)")
	return nil
}
