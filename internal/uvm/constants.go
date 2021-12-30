package uvm

import (
	"errors"
)

const (
	// MaxVPMEMCount is the maximum number of VPMem devices that may be added to an LCOW
	// utility VM
	MaxVPMEMCount = 128

	// DefaultVPMEMCount is the default number of VPMem devices that may be added to an LCOW
	// utility VM if the create request doesn't specify how many.
	DefaultVPMEMCount = 64

	// DefaultVPMemSizeBytes is the default size of a VPMem device if the create request
	// doesn't specify.
	DefaultVPMemSizeBytes = 4 * 1024 * 1024 * 1024 // 4GB

	// LCOWMountPathPrefix is the path format in the LCOW UVM where non global mounts, such
	// as Plan9 mounts are added
	LCOWMountPathPrefix = "/mounts/m%d"
	// LCOWGlobalMountPrefix is the path format in the LCOW UVM where global mounts are added
	LCOWGlobalMountPrefix = "/run/mounts/m%d"
	// LCOWNvidiaMountPath is the path format in LCOW UVM where nvidia tools are mounted
	// keep this value in sync with opengcs
	LCOWNvidiaMountPath = "/run/nvidia"
	// WCOWGlobalMountPrefix is the path prefix format in the WCOW UVM where mounts are added
	WCOWGlobalMountPrefix = "C:\\mounts\\m%d"
	// RootfsPath is part of the container's rootfs path
	RootfsPath = "rootfs"
)

var (
	errNotSupported = errors.New("not supported")
	errBadUVMOpts   = errors.New("UVM options incorrect")
)
