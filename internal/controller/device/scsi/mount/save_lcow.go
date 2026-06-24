//go:build windows && lcow

package mount

import (
	"fmt"

	scsisave "github.com/Microsoft/hcsshim/internal/controller/device/scsi/save"
)

// Save returns a migration snapshot of the mount. It fails unless the mount
// is mounted or reserved.
func (m *Mount) Save() (*scsisave.MountState, error) {
	if m.state != StateMounted && m.state != StateReserved {
		return nil, fmt.Errorf("scsi mount controller=%d lun=%d partition=%d in state %s; want %s", m.controller, m.lun, m.config.Partition, m.state, StateMounted)
	}
	return &scsisave.MountState{
		Config: &scsisave.MountConfig{
			ReadOnly:  m.config.ReadOnly,
			Encrypted: m.config.Encrypted,
			// Clone the slice so the snapshot does not alias live config.
			Options:          append([]string(nil), m.config.Options...),
			EnsureFilesystem: m.config.EnsureFilesystem,
			Filesystem:       m.config.Filesystem,
			BlockDev:         m.config.BlockDev,
		},
		RefCount:  uint32(m.refCount),
		GuestPath: m.guestPath,
	}, nil
}

// Import reconstructs a mount from a migration snapshot at the given controller,
// lun, and partition. It returns nil if the snapshot is nil.
func Import(state *scsisave.MountState, controller, lun uint, partition uint64) *Mount {
	if state == nil {
		return nil
	}

	// Rebuild the mount config from the snapshot, if present.
	cfg := Config{Partition: partition}
	if c := state.GetConfig(); c != nil {
		cfg.ReadOnly = c.GetReadOnly()
		cfg.Encrypted = c.GetEncrypted()
		cfg.Options = append([]string(nil), c.GetOptions()...)
		cfg.EnsureFilesystem = c.GetEnsureFilesystem()
		cfg.Filesystem = c.GetFilesystem()
		cfg.BlockDev = c.GetBlockDev()
	}
	// An imported mount is assumed to be live in the guest.
	return &Mount{
		controller: controller,
		lun:        lun,
		config:     cfg,
		state:      StateMounted,
		refCount:   int(state.GetRefCount()),
		guestPath:  state.GetGuestPath(),
	}
}
