//go:build functional

package functional

import (
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"
)

// default options using command line flags, if any

func getDefaultLcowUvmOptions(t *testing.T, name string) *uvm.OptionsLCOW {
	opts := uvm.NewDefaultOptionsLCOW(name, "")
	opts.BootFilesPath = *flagLinuxBootFilesPath

	return opts
}

func getDefaultWcowUvmOptions(t *testing.T, name string) *uvm.OptionsWCOW {
	opts := uvm.NewDefaultOptionsWCOW(name, "")

	return opts
}