//go:build windows

package uvm

import (
	"errors"

	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

var (
	errNotSupported = errors.New("not supported")
	errBadUVMOpts   = errors.New("UVM options incorrect")

	// Maximum number of SCSI controllers allowed
	MaxSCSIControllers = uint32(len(guestrequest.ScsiControllerGuids))
)
