package uvm

import (
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/sirupsen/logrus"
)

// TODO: This needs to move somewhere else.... CreateScratch for lcow is now in a separate package

// CreateWCOWScratch is a helper to create a scratch for a Windows utility VM
// with permissions to the specified VM ID in a specified directory
func CreateWCOWScratch(imagePath, destDirectory, vmID string) error {
	sourceScratch := filepath.Join(imagePath, `UtilityVM\SystemTemplate.vhdx`)
	targetScratch := filepath.Join(destDirectory, "sandbox.vhdx")
	logrus.Debugf("uvm::CreateWCOWScratch %s from %s", targetScratch, sourceScratch)
	if err := copyfile.CopyFile(sourceScratch, targetScratch, true); err != nil {
		return err
	}
	if err := wclayer.GrantVmAccess(vmID, targetScratch); err != nil {
		os.Remove(targetScratch)
		return err
	}
	return nil
}
