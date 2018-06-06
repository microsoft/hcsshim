package uvm

import "fmt"

const (
	// TODO: Tehse are moving to the lcow top-level package

	// DefaultLCOWScratchSizeGB is the size of the default LCOW scratch disk in GB
	DefaultLCOWScratchSizeGB = 20

	// defaultLCOWVhdxBlockSizeMB is the block-size for the scratch VHDx's this package can create.
	defaultLCOWVhdxBlockSizeMB = 1

	MaxVPMEM     = 128
	DefaultVPMEM = 64

	// TODO: These aren't actually used yet
	// When removing devices from a utility VM.
	removeTypeVirtualHardware = 1
	removeTypeNotifyGuest     = 2
	removeTypeAll             = removeTypeVirtualHardware + removeTypeNotifyGuest
)

var errNotSupported = fmt.Errorf("not supported")
