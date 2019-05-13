package svm

import (
	"fmt"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/lcow"
)

func (i *instance) createScratchNoLock(id string, sizeGB uint32, cacheDir string, targetDir string) error {
	// Keep a global service VM running - effectively a no-op
	if i.mode == ModeGlobal {
		id = globalID
	}

	// Nothing to do if no service VMs or not found
	if i.serviceVMs == nil {
		return ErrNotFound
	}
	svmItem, exists := i.serviceVMs[id]
	if !exists {
		return ErrNotFound
	}

	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("cache_ext4.%dGB.vhdx", sizeGB))
	targetFile := filepath.Join(targetDir, fmt.Sprintf("%s_svm_scratch.vhdx", id))
	return lcow.CreateScratch(svmItem.serviceVM.utilityVM, targetFile, sizeGB, cacheFile, id)
}

// CreateScratch generates a formatted EXT4 for use as a scratch disk.
func (i *instance) CreateScratch(id string, sizeGB uint32, cacheDir string, targetDir string) error {
	i.Lock()
	defer i.Unlock()
	return i.createScratchNoLock(id, sizeGB, cacheDir, targetDir)
}
