//go:build windows

package scsi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Microsoft/hcsshim/internal/wclayer"
)

var (
	// ErrNoAvailableLocation indicates that a new SCSI attachment failed because
	// no new slots were available.
	ErrNoAvailableLocation = errors.New("no available location")
	// ErrNotInitialized is returned when a method is invoked on a nil [Manager].
	ErrNotInitialized = errors.New("SCSI manager not initialized")
	// ErrAlreadyReleased is returned when [Mount.Release] is called on a Mount
	// that had already been released.
	ErrAlreadyReleased = errors.New("mount was already released")
)

// Manager is the primary entrypoint for managing SCSI devices on a VM.
// It tracks the state of what devices have been attached to the VM, and
// mounted inside the guest OS.
type Manager struct {
	attachManager *attachManager
	mountManager  *mountManager
}

// Slot represents a single SCSI slot, consisting of a controller and LUN.
type Slot struct {
	Controller uint
	LUN        uint
}

// NewManager creates a new Manager using the provided host and guest backends,
// as well as other configuration parameters.
//
// guestMountFmt is the format string to use for mounts of SCSI devices in
// the guest OS. It should have a single %d format parameter.
//
// reservedSlots indicates which SCSI slots to treat as already used. They
// will not be handed out again by the Manager.
func NewManager(
	hb HostBackend,
	gb GuestBackend,
	numControllers int,
	numLUNsPerController int,
	guestMountFmt string,
	reservedSlots []Slot,
) (*Manager, error) {
	if hb == nil || gb == nil {
		return nil, errors.New("host and guest backend must not be nil")
	}
	am := newAttachManager(hb, gb, numControllers, numLUNsPerController, reservedSlots)
	mm := newMountManager(gb, guestMountFmt)
	return &Manager{am, mm}, nil
}

// MountConfig specifies the options to apply for mounting a SCSI device in
// the guest OS.
type MountConfig struct {
	// Partition is the target partition index on a partitioned device to
	// mount. Partitions are 1-based indexed.
	// This is only supported for LCOW.
	Partition uint64
	// Encrypted indicates if we should encrypt the device with dm-crypt.
	// This is only supported for LCOW.
	Encrypted bool
	// Options are options such as propagation options, flags, or data to
	// pass to the mount call.
	// This is only supported for LCOW.
	Options []string
	// EnsureFilesystem indicates to format the mount as `Filesystem`
	// if it is not already formatted with that fs type.
	// This is only supported for LCOW.
	EnsureFilesystem bool
	// Filesystem is the target filesystem that a device will be
	// mounted as.
	// This is only supported for LCOW.
	Filesystem string
	// BlockDev indicates if the device should be mounted as a block device.
	// This is only supported for LCOW.
	BlockDev bool
	// FormatWithRefs indicates to refs format the disk.
	// This is only supported for CWCOW scratch disks.
	FormatWithRefs bool
}

// Mount represents a SCSI device that has been attached to a VM, and potentially
// also mounted into the guest OS.
type Mount struct {
	mgr         *Manager
	controller  uint
	lun         uint
	guestPath   string
	releaseOnce sync.Once
}

// Controller returns the controller number that the SCSI device is attached to.
func (m *Mount) Controller() uint {
	return m.controller
}

// LUN returns the LUN number that the SCSI device is attached to.
func (m *Mount) LUN() uint {
	return m.lun
}

// GuestPath returns the path inside the guest OS where the SCSI device was mounted.
// Will return an empty string if no guest mount was performed.
func (m *Mount) GuestPath() string {
	return m.guestPath
}

// Release releases the SCSI mount. Refcount tracking is used in case multiple instances
// of the same attachment or mount are used. If the refcount for the guest OS mount
// reaches 0, the guest OS mount is removed. If the refcount for the SCSI attachment
// reaches 0, the SCSI attachment is removed.
func (m *Mount) Release(ctx context.Context) (err error) {
	err = ErrAlreadyReleased
	m.releaseOnce.Do(func() {
		err = m.mgr.remove(ctx, m.controller, m.lun, m.guestPath)
	})
	return
}

// AddVirtualDisk attaches and mounts a VHD on the host to the VM. If the same
// VHD has already been attached to the VM, the existing attachment will
// be reused. If the same VHD has already been mounted in the guest OS
// with the same MountConfig, the same mount will be reused.
//
// If vmID is non-empty an ACL will be added to the VHD so that the specified VHD
// can access it.
//
// mc determines the settings to apply on the guest OS mount. If
// it is nil, no guest OS mount is performed.
func (m *Manager) AddVirtualDisk(
	ctx context.Context,
	hostPath string,
	readOnly bool,
	vmID string,
	guestPath string,
	mc *MountConfig,
) (*Mount, error) {
	if m == nil {
		return nil, ErrNotInitialized
	}
	if vmID != "" {
		if err := wclayer.GrantVmAccess(ctx, vmID, hostPath); err != nil {
			return nil, err
		}
	}
	var mcInternal *mountConfig
	if mc != nil {
		mcInternal = &mountConfig{
			partition:        mc.Partition,
			readOnly:         readOnly,
			encrypted:        mc.Encrypted,
			options:          mc.Options,
			ensureFilesystem: mc.EnsureFilesystem,
			filesystem:       mc.Filesystem,
			blockDev:         mc.BlockDev,
			formatWithRefs:   mc.FormatWithRefs,
		}
	}
	return m.add(ctx,
		&attachConfig{
			path:     hostPath,
			readOnly: readOnly,
			typ:      "VirtualDisk",
		},
		guestPath,
		mcInternal)
}

// AddPhysicalDisk attaches and mounts a physical disk on the host to the VM.
// If the same physical disk has already been attached to the VM, the existing
// attachment will be reused. If the same physical disk has already been mounted
// in the guest OS with the same MountConfig, the same mount will be reused.
//
// If vmID is non-empty an ACL will be added to the disk so that the specified VHD
// can access it.
//
// mc determines the settings to apply on the guest OS mount. If
// it is nil, no guest OS mount is performed.
func (m *Manager) AddPhysicalDisk(
	ctx context.Context,
	hostPath string,
	readOnly bool,
	vmID string,
	guestPath string,
	mc *MountConfig,
) (*Mount, error) {
	if m == nil {
		return nil, ErrNotInitialized
	}
	if vmID != "" {
		if err := wclayer.GrantVmAccess(ctx, vmID, hostPath); err != nil {
			return nil, err
		}
	}
	var mcInternal *mountConfig
	if mc != nil {
		mcInternal = &mountConfig{
			partition:        mc.Partition,
			readOnly:         readOnly,
			encrypted:        mc.Encrypted,
			options:          mc.Options,
			ensureFilesystem: mc.EnsureFilesystem,
			filesystem:       mc.Filesystem,
			blockDev:         mc.BlockDev,
		}
	}
	return m.add(ctx,
		&attachConfig{
			path:     hostPath,
			readOnly: readOnly,
			typ:      "PassThru",
		},
		guestPath,
		mcInternal)
}

// AddExtensibleVirtualDisk attaches and mounts an extensible virtual disk (EVD) to the VM.
// EVDs are made available by special drivers on the host which interact with the Hyper-V
// synthetic SCSI stack.
// If the same physical disk has already been attached to the VM, the existing
// attachment will be reused. If the same physical disk has already been mounted
// in the guest OS with the same MountConfig, the same mount will be reused.
//
// hostPath must adhere to the format "evd://<evdType>/<evdMountPath>".
//
// mc determines the settings to apply on the guest OS mount. If
// it is nil, no guest OS mount is performed.
func (m *Manager) AddExtensibleVirtualDisk(
	ctx context.Context,
	hostPath string,
	readOnly bool,
	guestPath string,
	mc *MountConfig,
) (*Mount, error) {
	if m == nil {
		return nil, ErrNotInitialized
	}
	evdType, mountPath, err := parseExtensibleVirtualDiskPath(hostPath)
	if err != nil {
		return nil, err
	}
	var mcInternal *mountConfig
	if mc != nil {
		mcInternal = &mountConfig{
			partition:        mc.Partition,
			readOnly:         readOnly,
			encrypted:        mc.Encrypted,
			options:          mc.Options,
			ensureFilesystem: mc.EnsureFilesystem,
			filesystem:       mc.Filesystem,
		}
	}
	return m.add(ctx,
		&attachConfig{
			path:     mountPath,
			readOnly: readOnly,
			typ:      "ExtensibleVirtualDisk",
			evdType:  evdType,
		},
		guestPath,
		mcInternal)
}

func (m *Manager) add(ctx context.Context, attachConfig *attachConfig, guestPath string, mountConfig *mountConfig) (_ *Mount, err error) {
	controller, lun, err := m.attachManager.attach(ctx, attachConfig)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_, _ = m.attachManager.detach(ctx, controller, lun)
		}
	}()

	if mountConfig != nil {
		guestPath, err = m.mountManager.mount(ctx, controller, lun, guestPath, mountConfig)
		if err != nil {
			return nil, err
		}
	}

	return &Mount{mgr: m, controller: controller, lun: lun, guestPath: guestPath}, nil
}

func (m *Manager) remove(ctx context.Context, controller, lun uint, guestPath string) error {
	if guestPath != "" {
		if err := m.mountManager.unmount(ctx, guestPath); err != nil {
			return err
		}
	}

	if _, err := m.attachManager.detach(ctx, controller, lun); err != nil {
		return err
	}

	return nil
}

// parseExtensibleVirtualDiskPath parses the evd path provided in the config.
// extensible virtual disk path has format "evd://<evdType>/<evd-mount-path>"
// this function parses that and returns the `evdType` and `evd-mount-path`.
func parseExtensibleVirtualDiskPath(hostPath string) (evdType, mountPath string, err error) {
	trimmedPath := strings.TrimPrefix(hostPath, "evd://")
	separatorIndex := strings.Index(trimmedPath, "/")
	if separatorIndex <= 0 {
		return "", "", fmt.Errorf("invalid extensible vhd path: %s", hostPath)
	}
	return trimmedPath[:separatorIndex], trimmedPath[separatorIndex+1:], nil
}
