package gcs

import (
	"github.com/Sirupsen/logrus"

	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/prot"
)

// CleanupContainer cleans up the state left behind by the container with the
// given ID.
// This function expects containerCacheMutex to be locked on entry.
func (c *gcsCore) CleanupContainer(id string) error {
	var errToReturn error
	if err := c.forceDeleteContainer(id); err != nil {
		logrus.Warn(err)
		if errToReturn == nil {
			errToReturn = err
		}
	}

	diskMap := c.containerCache[id].MappedVirtualDisks
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

	if err := c.unmountLayers(id); err != nil {
		logrus.Warn(err)
		if errToReturn == nil {
			errToReturn = err
		}
	}

	if err := c.destroyContainerStorage(id); err != nil {
		logrus.Warn(err)
		if errToReturn == nil {
			errToReturn = err
		}
	}

	return errToReturn
}

// forceDeleteContainer deletes the container, no matter its initial state.
func (c *gcsCore) forceDeleteContainer(id string) error {
	exists, err := c.Rtime.ContainerExists(id)
	if err != nil {
		return err
	}
	if exists {
		state, err := c.Rtime.GetContainerState(id)
		if err != nil {
			return err
		}
		status := state.Status
		// If the container is paused, resume it.
		if status == "paused" {
			if err := c.Rtime.ResumeContainer(id); err != nil {
				return err
			}
			status = "running"
		}
		if status == "running" {
			if err := c.Rtime.KillContainer(id, oslayer.SIGKILL); err != nil {
				return err
			}
			c.Rtime.WaitOnContainer(id)
		} else if status == "created" {
			// If we don't wait on a created container before deleting it, it
			// will become unblocked, and delete will fail.
			go c.Rtime.WaitOnContainer(id)
		}
		if err := c.Rtime.DeleteContainer(id); err != nil {
			return err
		}
	}
	return nil
}
