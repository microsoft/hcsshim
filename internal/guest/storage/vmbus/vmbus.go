// +build linux

package vmbus

import (
	"context"
	"path/filepath"

	"github.com/Microsoft/opengcs/internal/storage"
)

var storageWaitForFileMatchingPattern = storage.WaitForFileMatchingPattern

// WaitForDevicePath waits for the vmbus device to exist at /sys/bus/vmbus/devices/<vmbusGUIDPattern>...
func WaitForDevicePath(ctx context.Context, vmbusGUIDPattern string) (string, error) {
	vmBusPath := filepath.Join("/sys/bus/vmbus/devices", vmbusGUIDPattern)
	return storageWaitForFileMatchingPattern(ctx, vmBusPath)
}
