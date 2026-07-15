//go:build windows && (lcow || wcow)

package disk

import (
	"fmt"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	scsisave "github.com/Microsoft/hcsshim/internal/controller/device/scsi/save"
)

// Save returns a migration snapshot of the disk and its mounts. It fails unless
// the disk is attached or reserved and every mount can be saved.
func (d *Disk) Save() (*scsisave.DiskState, error) {
	if d.state != StateAttached && d.state != StateReserved {
		return nil, fmt.Errorf("scsi disk controller=%d lun=%d in state %s; want %s", d.controller, d.lun, d.state, StateAttached)
	}

	out := &scsisave.DiskState{
		Config: &scsisave.DiskConfig{
			HostPath: d.config.HostPath,
			ReadOnly: d.config.ReadOnly,
			Type:     string(d.config.Type),
			EvdType:  d.config.EVDType,
		},
	}

	if len(d.mounts) > 0 {
		out.Mounts = make(map[uint64]*scsisave.MountState, len(d.mounts))

		// Snapshot every mount; abort if any cannot be saved.
		for partition, m := range d.mounts {
			ms, err := m.Save()
			if err != nil {
				return nil, err
			}
			out.Mounts[partition] = ms
		}
	}
	return out, nil
}

// Import reconstructs a disk and its mounts from a migration snapshot at the
// given controller and lun. It returns nil if the snapshot is nil.
func Import(state *scsisave.DiskState, controller, lun uint) *Disk {
	if state == nil {
		return nil
	}

	// Rebuild the host-side config from the snapshot, if present.
	cfg := Config{}
	if c := state.GetConfig(); c != nil {
		cfg = Config{
			HostPath: c.GetHostPath(),
			ReadOnly: c.GetReadOnly(),
			Type:     Type(c.GetType()),
			EVDType:  c.GetEvdType(),
		}
	}

	// An imported disk is assumed to be live on the SCSI bus.
	d := &Disk{
		controller: controller,
		lun:        lun,
		config:     cfg,
		state:      StateAttached,
		mounts:     make(map[uint64]*mount.Mount, len(state.GetMounts())),
	}

	// Reconstruct each partition mount, skipping any that fail to import.
	for partition, ms := range state.GetMounts() {
		m := mount.Import(ms, controller, lun, partition)
		if m == nil {
			continue
		}
		d.mounts[partition] = m
	}
	return d
}

// UpdateHostPath rewrites the host-side path of the disk image.
func (d *Disk) UpdateHostPath(p string) {
	d.config.HostPath = p
}
