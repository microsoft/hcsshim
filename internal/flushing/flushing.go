package flushing

// This package deals with VHD/registry flushing in the v2 HCS schema where
// on RS5..18853 don't implement the optimizations

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/Microsoft/go-winio/vhd"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	//"golang.org/x/sys/windows"
)

type writeCacheMode uint16

const (
	// Write Cache Mode for a VHD.
	writeCacheModeCacheMetadata         writeCacheMode = 0
	writeCacheModeWriteInternalMetadata writeCacheMode = 1
	writeCacheModeWriteMetadata         writeCacheMode = 2
	writeCacheModeCommitAll             writeCacheMode = 3
	writeCacheModeDisableFlushing       writeCacheMode = 4
)

// setVhdWriteCacheMode sets the WriteCacheMode for a VHD. The handle
// to the VHD should be opened with Access: None, Flags: ParentCachedIO |
// IgnoreRelativeParentLocator. Use DisableFlushing for optimisation during
// first boot, and CacheMetadata following container start
func setVhdWriteCacheMode(handle syscall.Handle, wcm writeCacheMode) error {
	type storageSetSurfaceCachePolicyRequest struct {
		RequestLevel uint32
		CacheMode    uint16
		pad          uint16 // For 4-byte alignment
	}
	const ioctlSetSurfaceCachePolicy uint32 = 0x2d1a10
	request := storageSetSurfaceCachePolicyRequest{
		RequestLevel: 1,
		CacheMode:    uint16(wcm),
		pad:          0,
	}
	var bytesReturned uint32
	return syscall.DeviceIoControl(
		handle,
		ioctlSetSurfaceCachePolicy,
		(*byte)(unsafe.Pointer(&request)),
		uint32(unsafe.Sizeof(request)),
		nil,
		0,
		&bytesReturned,
		nil)
}

// PreStartFlushDisable conditionally disables flushing if required
// prior to the computesystem Start() call. This is necessary only on
// certain Windows builds where HCS does not implement this functionality
// itself.
func PreStartFlushDisable(
	isWCOW bool,
	ignoreFlushes bool,
	isHyperV bool,
	tid string,
	eid string,
	scratch string) (syscall.Handle, error) {

	// No-op pre-RS5 or post-18855. Pre-RS5 doesn't use v2. Post 18855 has
	// these optimisations in the platform for v2 callers. Only for WCOW.
	osv := osversion.Get()
	if osv.Build < osversion.RS5 || osv.Build >= 18855 ||
		!isWCOW ||
		!ignoreFlushes ||
		isHyperV { // TODO @jhowardmsft Remove this when xenon WCOW bit implemented
		return 0, nil
	}

	if !isHyperV {
		// Operating on the scratch disk
		//path := filepath.Join(he.spec.Windows.LayerFolders[len(he.spec.Windows.LayerFolders)-1], "sandbox.vhdx")

		logrus.WithFields(logrus.Fields{
			"tid":     tid,
			"eid":     eid,
			"scratch": scratch,
		}).Debug("Disabling VHD flushing")

		handle, err := vhd.OpenVirtualDisk(scratch, vhd.VirtualDiskAccessNone, vhd.OpenVirtualDiskFlagParentCachedIO|vhd.OpenVirtualDiskFlagIgnoreRelativeParentLocator)
		if err != nil {
			syscall.CloseHandle(handle)
			return 0, errors.Wrap(err, fmt.Sprintf("failed to open %s", scratch))
		}
		if err := setVhdWriteCacheMode(handle, writeCacheModeDisableFlushing); err != nil {
			syscall.CloseHandle(handle)
			return 0, errors.Wrap(err, fmt.Sprintf("failed to disable flushing on %s", scratch))
		}
		return handle, nil

	}

	// TODO @jhowardmsft - Extend for xenon WCOW
	return 0, nil
}

// PostStartFlushEnable conditionally enables flushing if required
// after the computesystem Start() call. It effectively reverses anything
// that might have been done in `PreStartFlushDisable()`.
func PostStartFlushEnable(handle syscall.Handle, isHyperV bool, tid string, eid string) {
	if !isHyperV {
		if handle == 0 {
			return
		}

		logrus.WithFields(logrus.Fields{
			"tid": tid,
			"eid": eid,
		}).Debug("Re-enabling VHD flushing")

		setVhdWriteCacheMode(handle, writeCacheModeCacheMetadata)
		syscall.CloseHandle(handle)
	}

	// TODO @jhowardmsft - Extend for xenon WCOW

}
