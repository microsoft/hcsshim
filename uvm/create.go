package uvm

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/guid"
	"github.com/sirupsen/logrus"
)

// Create creates an HCS compute system representing a utility VM.
//
// WCOW Notes:
//   - If the sandbox folder does not exist, it will be created
//   - If the sandbox folder does not contain `sandbox.vhdx` it will be created based on the system template located in the layer folders.
//   - The sandbox is always attached to SCSI 0:0
//
func Create(opts *UVMOptions) (*UtilityVM, error) {
	logrus.Debugf("uvm::Create %+v", opts)

	uvm := &UtilityVM{
		id:              opts.Id,
		owner:           opts.Owner,
		operatingSystem: opts.OperatingSystem,
	}

	if opts.OperatingSystem != "linux" && opts.OperatingSystem != "windows" {
		logrus.Debugf("uvm::Create Unsupported OS")
		return nil, fmt.Errorf("unsupported operating system %q", opts.OperatingSystem)
	}

	// Defaults if omitted by caller.
	if uvm.id == "" {
		uvm.id = guid.New().String()
	}
	if uvm.owner == "" {
		uvm.owner = filepath.Base(os.Args[0])
	}

	if uvm.operatingSystem == "windows" {
		logrus.Debugf("uvm::Create Windows utility VM")
		if err := uvm.createWCOW(opts); err != nil {
			return nil, err
		}
	} else {
		logrus.Debugf("uvm::Create Linux utility VM")
		if err := uvm.createLCOW(opts); err != nil {
			return nil, err
		}
	}
	return uvm, nil
}
