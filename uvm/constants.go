package uvm

import "fmt"

const (
	// DefaultLCOWSandboxSizeGB is the size of the default LCOW sandbox in GB
	DefaultLCOWSandboxSizeGB = 20

	// defaultLCOWVhdxBlockSizeMB is the block-size for the sandbox/scratch VHDx's this package can create.
	defaultLCOWVhdxBlockSizeMB = 1

	maxVPMEM = 128

	// When removing devices from a utility VM.
	removeTypeVirtualHardware = 1
	removeTypeNotifyGuest     = 2
	removeTypeAll             = removeTypeVirtualHardware + removeTypeNotifyGuest
)

var errNotSupported = fmt.Errorf("not supported")
