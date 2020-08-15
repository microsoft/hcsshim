package uvm

import (
	"context"
	"os"

	"github.com/Microsoft/hcsshim/internal/log"
)

// AutoManagedVHD struct representing a VHD that will be cleaned up automatically.
type AutoManagedVHD struct {
	hostPath string
}

func NewAutoManagedVHD(hostPath string) *AutoManagedVHD {
	return &AutoManagedVHD{
		hostPath: hostPath,
	}
}

// Release removes the vhd.
func (vhd *AutoManagedVHD) Release(ctx context.Context) error {
	if err := os.Remove(vhd.hostPath); err != nil {
		log.G(ctx).WithField("hostPath", vhd.hostPath).WithError(err).Error("failed to remove automanage-virtual-disk")
	}
	return nil
}
