//go:build linux
// +build linux

package hcsv2

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

type rwDevice struct {
	mountPath  string
	sourcePath string
	encrypted  bool
}

type hostMounts struct {
	stateMutex sync.Mutex

	// Holds information about read-write devices, which can be encrypted and
	// contain overlay fs upper/work directory mounts.
	readWriteMounts map[string]*rwDevice
}

func newHostMounts() *hostMounts {
	return &hostMounts{
		readWriteMounts: map[string]*rwDevice{},
	}
}

// AddRWDevice adds read-write device metadata for device mounted at `mountPath`.
// Returns an error if there's an existing device mounted at `mountPath` location.
func (hm *hostMounts) AddRWDevice(mountPath string, sourcePath string, encrypted bool) error {
	hm.stateMutex.Lock()
	defer hm.stateMutex.Unlock()

	mountTarget := filepath.Clean(mountPath)
	if source, ok := hm.readWriteMounts[mountTarget]; ok {
		return fmt.Errorf("read-write with source %q and mount target %q already exists", source.sourcePath, mountPath)
	}
	hm.readWriteMounts[mountTarget] = &rwDevice{
		mountPath:  mountTarget,
		sourcePath: sourcePath,
		encrypted:  encrypted,
	}
	return nil
}

// RemoveRWDevice removes the read-write device metadata for device mounted at
// `mountPath`.
func (hm *hostMounts) RemoveRWDevice(mountPath string, sourcePath string) error {
	hm.stateMutex.Lock()
	defer hm.stateMutex.Unlock()

	unmountTarget := filepath.Clean(mountPath)
	device, ok := hm.readWriteMounts[unmountTarget]
	if !ok {
		// already removed or didn't exist
		return nil
	}
	if device.sourcePath != sourcePath {
		return fmt.Errorf("wrong sourcePath %s", sourcePath)
	}

	delete(hm.readWriteMounts, unmountTarget)
	return nil
}

// IsEncrypted checks if the given path is a sub-path of an encrypted read-write
// device.
func (hm *hostMounts) IsEncrypted(path string) bool {
	hm.stateMutex.Lock()
	defer hm.stateMutex.Unlock()

	parentPath := ""
	encrypted := false
	cleanPath := filepath.Clean(path)
	for rwPath, rwDev := range hm.readWriteMounts {
		relPath, err := filepath.Rel(rwPath, cleanPath)
		// skip further checks if an error is returned or the relative path
		// contains "..", meaning that the `path` isn't directly nested under
		// `rwPath`.
		if err != nil || strings.HasPrefix(relPath, "..") {
			continue
		}
		if len(rwDev.mountPath) > len(parentPath) {
			parentPath = rwDev.mountPath
			encrypted = rwDev.encrypted
		}
	}
	return encrypted
}
