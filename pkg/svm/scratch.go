package svm

import (
	"fmt"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/lcow"
)

// CreateScratch generates a formatted EXT4 for use as a scratch disk.
func (i *instance) CreateScratch(id string, sizeGB uint32, cacheDir string, targetDir string) error {

	// Keep a global service VM running - effectively a no-op
	if i.mode == ModeGlobal {
		id = globalID
	}

	i.Lock()
	defer i.Unlock()

	// Nothing to do if no service VMs or not found
	if i.serviceVMs == nil {
		return ErrNotFound
	}
	svmItem, exists := i.serviceVMs[id]
	if !exists {
		return ErrNotFound
	}

	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("scratch.%d.vhdx", sizeGB))
	targetFile := filepath.Join(targetDir, "scratch.vhdx")
	return lcow.CreateScratch(svmItem.serviceVM.utilityVM, targetFile, sizeGB, cacheFile, id)
}
