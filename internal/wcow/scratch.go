package wcow

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/sirupsen/logrus"
)

// CreateUVMScratch is a helper to create a scratch for a Windows utility VM
// with permissions to the specified VM ID in a specified directory
func CreateUVMScratch(ctx context.Context, imagePath, destDirectory, vmID string) error {
	sourceScratch := filepath.Join(imagePath, `UtilityVM\SystemTemplate.vhdx`)
	targetScratch := filepath.Join(destDirectory, "sandbox.vhdx")
	log.G(ctx).WithFields(logrus.Fields{
		"target": targetScratch,
		"source": sourceScratch,
	}).Debug("uvm::CreateUVMScratch")
	if err := copyfile.CopyFile(sourceScratch, targetScratch, true); err != nil {
		return err
	}
	if err := wclayer.GrantVmAccess(vmID, targetScratch); err != nil {
		os.Remove(targetScratch)
		return err
	}
	return nil
}
