package uvm

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Microsoft/go-winio/vhd"
	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

// CreateLCOWSandbox uses a utility VM to create an empty Sandbox disk of a requested size.
// It has a caching capability. If the cacheFile exists, and the request is for a default
// size, a copy of that is made to the target. If the size is non-default, or the cache file
// does not exist, it uses a utility VM to create target. It is the responsibility of the
// caller to synchronise simultaneous attempts to create the cache file.
func (uvm *UtilityVM) CreateLCOWSandbox(destFile string, sizeGB uint32, cacheFile string, vmID string) error {
	if uvm == nil {
		return fmt.Errorf("no utility VM")
	}

	if uvm.operatingSystem != "linux" {
		return fmt.Errorf("CreateLCOWSandbox requires a linux utility VM to operate!")
	}

	// Smallest we can accept is the default sandbox size as we can't size down, only expand.
	if sizeGB < DefaultLCOWSandboxSizeGB {
		sizeGB = DefaultLCOWSandboxSizeGB
	}

	logrus.Debugf("hcsshim::CreateLCOWSandbox: Dest:%s size:%dGB cache:%s", destFile, sizeGB, cacheFile)

	// Retrieve from cache if the default size and already on disk
	if cacheFile != "" && sizeGB == DefaultLCOWSandboxSizeGB {
		if _, err := os.Stat(cacheFile); err == nil {
			if err := copyfile.CopyFile(cacheFile, destFile, false); err != nil {
				return fmt.Errorf("failed to copy cached file '%s' to '%s': %s", cacheFile, destFile, err)
			}
			logrus.Debugf("hcsshim::CreateLCOWSandbox: %s fulfilled from cache (%s)", destFile, cacheFile)

			if vmID != "" {
				if err := wclayer.GrantVmAccess(vmID, destFile); err != nil {
					os.Remove(destFile)
					return err
				}
			}

			return nil
		}
	}

	if uvm == nil {
		return fmt.Errorf("cannot create Sandbox disk as cache is not present and no utility VM supplied")
	}

	// Create the VHDX
	if err := vhd.CreateVhdx(destFile, sizeGB, defaultLCOWVhdxBlockSizeMB); err != nil {
		return fmt.Errorf("failed to create VHDx %s: %s", destFile, err)
	}

	//	uvmc.DebugLCOWGCS()
	// Grant access
	if err := wclayer.GrantVmAccess(uvm.id, destFile); err != nil {
		return err
	}

	controller, lun, err := uvm.AddSCSI(destFile, "") // No destination as not formatted
	if err != nil {
		return err
	}

	logrus.Debugf("hcsshim::CreateLCOWSandbox: %s at C=%d L=%d", destFile, controller, lun)

	// Validate /sys/bus/scsi/devices/C:0:0:L exists as a directory
	testdCommand := []string{"test", "-d", fmt.Sprintf("/sys/bus/scsi/devices/%d:0:0:%d", controller, lun)}
	testdProc, _, err := uvm.CreateProcess(&ProcessOptions{Process: &specs.Process{Args: testdCommand}})
	if err != nil {
		uvm.removeSCSI(destFile, "", controller, lun)
		return fmt.Errorf("failed to run %+v following hot-add %s to utility VM: %s", testdCommand, destFile, err)
	}
	defer testdProc.Close()

	testdProc.WaitTimeout(defaultTimeoutSeconds)
	testdExitCode, err := testdProc.ExitCode()
	if err != nil {
		uvm.removeSCSI(destFile, "", controller, lun)
		return fmt.Errorf("failed to get exit code from from %+v following hot-add %s to utility VM: %s", testdCommand, destFile, err)
	}
	if testdExitCode != 0 {
		uvm.removeSCSI(destFile, "", controller, lun)
		return fmt.Errorf("`%+v` return non-zero exit code (%d) following hot-add %s to utility VM", testdCommand, testdExitCode, destFile)
	}

	// Get the device from under the block subdirectory by doing a simple ls. This will come back as (eg) `sda`
	var lsOutput bytes.Buffer
	lsCommand := []string{"ls", fmt.Sprintf("/sys/bus/scsi/devices/%d:0:0:%d/block", controller, lun)}
	lsProc, _, err := uvm.CreateProcess(&ProcessOptions{
		Process: &specs.Process{Args: lsCommand},
		Stdout:  &lsOutput,
	})
	if err != nil {
		uvm.removeSCSI(destFile, "", controller, lun)
		return fmt.Errorf("failed to `%+v` following hot-add %s to utility VM: %s", lsCommand, destFile, err)
	}
	defer lsProc.Close()
	lsProc.WaitTimeout(defaultTimeoutSeconds)
	lsExitCode, err := lsProc.ExitCode()
	if err != nil {
		uvm.removeSCSI(destFile, "", controller, lun)
		return fmt.Errorf("failed to get exit code from `%+v` following hot-add %s to utility VM: %s", lsCommand, destFile, err)
	}
	if lsExitCode != 0 {
		uvm.removeSCSI(destFile, "", controller, lun)
		return fmt.Errorf("`%+v` return non-zero exit code (%d) following hot-add %s to utility VM", lsCommand, lsExitCode, destFile)
	}
	device := fmt.Sprintf(`/dev/%s`, strings.TrimSpace(lsOutput.String()))
	logrus.Debugf("hcsshim: CreateExt4Vhdx: %s: device at %s", destFile, device)

	// Format it ext4
	mkfsCommand := []string{"mkfs.ext4", "-q", "-E", "lazy_itable_init=1", "-O", `^has_journal,sparse_super2,uninit_bg,^resize_inode`, device}
	var mkfsStderr bytes.Buffer
	mkfsProc, _, err := uvm.CreateProcess(&ProcessOptions{
		Process: &specs.Process{Args: mkfsCommand},
		Stderr:  &mkfsStderr,
	})
	if err != nil {
		uvm.removeSCSI(destFile, "", controller, lun)
		return fmt.Errorf("failed to `%+v` following hot-add %s to utility VM: %s", mkfsCommand, destFile, err)
	}
	defer mkfsProc.Close()
	mkfsProc.WaitTimeout(defaultTimeoutSeconds)
	mkfsExitCode, err := mkfsProc.ExitCode()
	if err != nil {
		uvm.removeSCSI(destFile, "", controller, lun)
		return fmt.Errorf("failed to get exit code from `%+v` following hot-add %s to utility VM: %s", mkfsCommand, destFile, err)
	}
	if mkfsExitCode != 0 {
		uvm.removeSCSI(destFile, "", controller, lun)
		return fmt.Errorf("`%+v` return non-zero exit code (%d) following hot-add %s to utility VM: %s", mkfsCommand, mkfsExitCode, destFile, strings.TrimSpace(mkfsStderr.String()))
	}

	// Hot-Remove before we copy it
	if err := uvm.removeSCSI(destFile, "", controller, lun); err != nil {
		return fmt.Errorf("failed to hot-remove: %s", err)
	}

	// Populate the cache.
	if cacheFile != "" && (sizeGB == DefaultLCOWSandboxSizeGB) {
		if err := copyfile.CopyFile(destFile, cacheFile, true); err != nil {
			return fmt.Errorf("failed to seed cache '%s' from '%s': %s", destFile, cacheFile, err)
		}
	}

	if vmID != "" {
		if err := wclayer.GrantVmAccess(vmID, destFile); err != nil {
			os.Remove(destFile)
			return err
		}
	}

	logrus.Debugf("hcsshim::CreateLCOWSandbox: %s created (non-cache)", destFile)
	return nil
}

// CreateWCOWSandbox is a helper to create a sandbox for a Windows utility VM
// with permissions to the specified VM ID in a specified directory
func CreateWCOWSandbox(imagePath, destDirectory, vmID string) error {
	sourceSandbox := filepath.Join(imagePath, `UtilityVM\SystemTemplate.vhdx`)
	targetSandbox := filepath.Join(destDirectory, "sandbox.vhdx")
	logrus.Debugf("uvm::CreateWCOWSandbox %s from %s", targetSandbox, sourceSandbox)
	if err := copyfile.CopyFile(sourceSandbox, targetSandbox, true); err != nil {
		return err
	}
	if err := wclayer.GrantVmAccess(vmID, targetSandbox); err != nil {
		os.Remove(targetSandbox)
		return err
	}
	return nil
}
