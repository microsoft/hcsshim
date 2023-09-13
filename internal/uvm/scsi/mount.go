package scsi

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"
)

type mountManager struct {
	m       sync.Mutex
	mounter mounter
	// Tracks current mounts. Entries will be nil if the mount was unmounted, meaning the index is
	// available for use.
	mounts   []*mount
	mountFmt string
}

func newMountManager(mounter mounter, mountFmt string) *mountManager {
	return &mountManager{
		mounter:  mounter,
		mountFmt: mountFmt,
	}
}

type mount struct {
	path       string
	index      int
	controller uint
	lun        uint
	config     *mountConfig
	waitErr    error
	waitCh     chan struct{}
	refCount   uint
}

type mountConfig struct {
	partition        uint64
	readOnly         bool
	encrypted        bool
	options          []string
	ensureFilesystem bool
	filesystem       string
}

func (mm *mountManager) mount(ctx context.Context, controller, lun uint, c *mountConfig) (_ string, err error) {
	// Normalize the mount config for comparison.
	// Config equality relies on the options slices being compared element-wise. Sort the options
	// slice first so that two slices with different ordering compare as equal. We assume that
	// order will never matter for mount options.
	sort.Strings(c.options)

	mount, existed := mm.trackMount(controller, lun, c)
	if existed {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-mount.waitCh:
			if mount.waitErr != nil {
				return "", mount.waitErr
			}
		}
		return mount.path, nil
	}

	defer func() {
		if err != nil {
			mm.m.Lock()
			mm.untrackMount(mount)
			mm.m.Unlock()
		}

		mount.waitErr = err
		close(mount.waitCh)
	}()

	if err := mm.mounter.mount(ctx, controller, lun, mount.path, c); err != nil {
		return "", fmt.Errorf("mount scsi controller %d lun %d at %s: %w", controller, lun, mount.path, err)
	}
	return mount.path, nil
}

func (mm *mountManager) unmount(ctx context.Context, path string) (bool, error) {
	mm.m.Lock()
	defer mm.m.Unlock()

	var mount *mount
	for _, mount = range mm.mounts {
		if mount != nil && mount.path == path {
			break
		}
	}

	mount.refCount--
	if mount.refCount > 0 {
		return false, nil
	}

	if err := mm.mounter.unmount(ctx, mount.controller, mount.lun, mount.path, mount.config); err != nil {
		return false, fmt.Errorf("unmount scsi controller %d lun %d at path %s: %w", mount.controller, mount.lun, mount.path, err)
	}
	mm.untrackMount(mount)

	return true, nil
}

func (mm *mountManager) trackMount(controller, lun uint, c *mountConfig) (*mount, bool) {
	mm.m.Lock()
	defer mm.m.Unlock()

	var freeIndex int = -1
	for i, mount := range mm.mounts {
		if mount == nil {
			if freeIndex == -1 {
				freeIndex = i
			}
		} else if controller == mount.controller &&
			lun == mount.lun &&
			reflect.DeepEqual(c, mount.config) {

			mount.refCount++
			return mount, true
		}
	}

	// New mount.
	mount := &mount{
		controller: controller,
		lun:        lun,
		config:     c,
		refCount:   1,
		waitCh:     make(chan struct{}),
	}
	if freeIndex == -1 {
		mount.index = len(mm.mounts)
		mm.mounts = append(mm.mounts, mount)
	} else {
		mount.index = freeIndex
		mm.mounts[freeIndex] = mount
	}
	// Use the mount index to produce a unique guest path.
	mount.path = fmt.Sprintf(mm.mountFmt, mount.index)
	return mount, false
}

// Caller must be holding mm.m.
func (mm *mountManager) untrackMount(mount *mount) {
	mm.mounts[mount.index] = nil
}
