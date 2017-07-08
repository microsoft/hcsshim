package gcs

import (
	"github.com/Sirupsen/logrus"

	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
)

// CleanupContainer cleans up the state left behind by the container with the
// given ID.
// This function expects containerCacheMutex to be locked on entry.
func (c *gcsCore) cleanupContainer(containerEntry *containerCacheEntry) error {
	var errToReturn error
	if err := c.forceDeleteContainer(containerEntry.container); err != nil {
		logrus.Warn(err)
		if errToReturn == nil {
			errToReturn = err
		}
	}

	diskMap := containerEntry.MappedVirtualDisks
	disks := make([]prot.MappedVirtualDisk, 0, len(diskMap))
	for _, disk := range diskMap {
		disks = append(disks, disk)
	}
	if err := c.unmountMappedVirtualDisks(disks); err != nil {
		logrus.Warn(err)
		if errToReturn == nil {
			errToReturn = err
		}
	}

	if err := c.unmountLayers(containerEntry.ID); err != nil {
		logrus.Warn(err)
		if errToReturn == nil {
			errToReturn = err
		}
	}

	if err := c.destroyContainerStorage(containerEntry.ID); err != nil {
		logrus.Warn(err)
		if errToReturn == nil {
			errToReturn = err
		}
	}

	return errToReturn
}

// forceDeleteContainer deletes the container, no matter its initial state.
func (c *gcsCore) forceDeleteContainer(container runtime.Container) error {
	exists, err := container.Exists()
	if err != nil {
		return err
	}
	if exists {
		state, err := container.GetState()
		if err != nil {
			return err
		}
		status := state.Status
		// If the container is paused, resume it.
		if status == "paused" {
			if err := container.Resume(); err != nil {
				return err
			}
			status = "running"
		}
		if status == "running" {
			if err := container.Kill(oslayer.SIGKILL); err != nil {
				return err
			}
			container.Wait()
		} else if status == "created" {
			// If we don't wait on a created container before deleting it, it
			// will become unblocked, and delete will fail.
			go container.Wait()
		}
		if err := container.Delete(); err != nil {
			return err
		}
	}
	return nil
}
