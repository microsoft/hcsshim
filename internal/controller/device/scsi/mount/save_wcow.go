//go:build windows && wcow

package mount

import scsisave "github.com/Microsoft/hcsshim/internal/controller/device/scsi/save"

// Save is a WCOW no-op stub as of now.
func (m *Mount) Save() (*scsisave.MountState, error) {
	return &scsisave.MountState{}, nil
}

// Import is a WCOW no-op stub as of now.
func Import(_ *scsisave.MountState, _, _ uint, _ uint64) *Mount {
	return &Mount{}
}
