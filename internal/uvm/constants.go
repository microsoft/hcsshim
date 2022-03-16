package uvm

import (
	"errors"

	guid "github.com/Microsoft/go-winio/pkg/guid"
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

	// V5 GUIDs for SCSI controllers
	// These GUIDs are created with namespace GUID "d422512d-2bf2-4752-809d-7b82b5fcb1b4"
	// and index as names. For example, first GUID is created like this:
	// guid.NewV5("d422512d-2bf2-4752-809d-7b82b5fcb1b4", []byte("0"))

	ScsiControllerGuids = []guid.GUID{
		// df6d0690-79e5-55b6-a5ec-c1e2f77f580a
		{
			Data1: 0xdf6d0690,
			Data2: 0x79e5,
			Data3: 0x55b6,
			Data4: [8]byte{0xa5, 0xec, 0xc1, 0xe2, 0xf7, 0x7f, 0x58, 0x0a},
		},
		// 0110f83b-de10-5172-a266-78bca56bf50a
		{
			Data1: 0x0110f83b,
			Data2: 0xde10,
			Data3: 0x5172,
			Data4: [8]byte{0xa2, 0x66, 0x78, 0xbc, 0xa5, 0x6b, 0xf5, 0x0a},
		},
		// b5d2d8d4-3a75-51bf-945b-3444dc6b8579
		{
			Data1: 0xb5d2d8d4,
			Data2: 0x3a75,
			Data3: 0x51bf,
			Data4: [8]byte{0x94, 0x5b, 0x34, 0x44, 0xdc, 0x6b, 0x85, 0x79},
		},
		// 305891a9-b251-5dfe-91a2-c25d9212275b
		{
			Data1: 0x305891a9,
			Data2: 0xb251,
			Data3: 0x5dfe,
			Data4: [8]byte{0x91, 0xa2, 0xc2, 0x5d, 0x92, 0x12, 0x27, 0x5b},
		},
	}

	// Maximum number of SCSI controllers allowed
	MaxSCSIControllers = uint32(len(ScsiControllerGuids))
)
