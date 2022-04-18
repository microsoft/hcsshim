package uvm

import (
	"errors"

	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
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
	DefaultVPMemSizeBytes = 4 * memory.GiB // 4GB
)

var (
	errNotSupported = errors.New("not supported")
	errBadUVMOpts   = errors.New("UVM options incorrect")

	// Maximum number of SCSI controllers allowed
	MaxSCSIControllers = uint32(len(guestrequest.ScsiControllerGuids))
)
