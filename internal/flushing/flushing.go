package flushing

// This package deals with VHD/registry flushing in the v2 HCS schema where
// on RS5..18853 don't implement the optimizations

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/Microsoft/go-winio/vhd"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type writeCacheMode uint16

const (
	// Write cache modes for a VHD.
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

// getFlushDisableSettings determines whether or not we are doing
// VHD flushing and/or registry flushing disablement.
func getFlushDisableSettings(isWCOW, ignoreFlushes bool) (vhdOpt bool, registryOpt bool) {

	vhdOpt = false
	registryOpt = false

	// Pre-RS5 doesn't use v2. Post 18855 has registry flush capability
	// in the platform for v2 callers. All of this is WCOW specific, and
	// requires that the OCI spec being used indicates flushes should be
	// ignored during boot.
	osv := osversion.Get()
	if osv.Build < osversion.RS5 || !isWCOW || !ignoreFlushes {
		return
	}

	vhdOpt = true
	if osv.Build > 18855 {
		registryOpt = true
	}
	return
}

// PreStartFlushDisable conditionally disables flushing if required
// prior to the computesystem Start() call. This is necessary only on
// certain Windows builds where HCS does not implement this functionality
// itself. It returns a handle (argon case) to the sandbox VHD which
// should be passed into PostStartFlushEnable after Start() has completed.
func PreStartFlushDisable(
	isWCOW bool,
	ignoreFlushes bool,
	c *hcs.System,
	host *uvm.UtilityVM,
	tid string,
	eid string,
	scratch string) (handle syscall.Handle, err error) {

	vhdOpt, registryOpt := getFlushDisableSettings(isWCOW, ignoreFlushes)
	if !vhdOpt && !registryOpt {
		return 0, nil
	}

	registryOptDone := false
	vhdOptDone := false

	defer func() {
		if err != nil {

			if registryOptDone {
				if innerErr := modifyRegistryFlushState(c, true); innerErr != nil {
					logrus.WithFields(logrus.Fields{
						"tid": tid,
						"eid": eid,
					}).WithError(innerErr).Error("failed to rollback registry flushing following previous error")
				}
			}

			if vhdOptDone {
				if innerErr := setVhdWriteCacheMode(handle, writeCacheModeCacheMetadata); innerErr != nil {
					logrus.WithFields(logrus.Fields{
						"tid":     tid,
						"eid":     eid,
						"scratch": scratch,
					}).WithError(innerErr).Error("failed to rollback VHD write cache mode following previous error")

				}
				syscall.CloseHandle(handle)
				handle = 0
			}
		}
	}()

	if vhdOpt {
		logrus.WithFields(logrus.Fields{
			"tid":     tid,
			"eid":     eid,
			"scratch": scratch,
		}).Debug("Disabling VHD flushing")

		// Note it is safe to go direct to the disk here, even in the Xenon case.
		if handle, err = vhd.OpenVirtualDisk(scratch, vhd.VirtualDiskAccessNone, vhd.OpenVirtualDiskFlagParentCachedIO|vhd.OpenVirtualDiskFlagIgnoreRelativeParentLocator); err != nil {
			err = errors.Wrapf(err, "failed to open %s", scratch)
			return
		}
		if err = setVhdWriteCacheMode(handle, writeCacheModeDisableFlushing); err != nil {
			err = errors.Wrapf(err, "failed to disable flushing on %s", scratch)
			return
		}
		vhdOptDone = true
	}

	if registryOpt {
		logrus.WithFields(logrus.Fields{
			"tid": tid,
			"eid": eid,
		}).Debug("Disabling registry flushing")

		if err = modifyRegistryFlushState(c, false); err != nil {
			err = errors.Wrapf(err, "failed to disable registry flushing")
			return
		}
		registryOptDone = true
	}

	return
}

// PostStartFlushEnable conditionally enables flushing if required
// after the computesystem Start() call. It effectively reverses anything
// that might have been done in `PreStartFlushDisable()`.
func PostStartFlushEnable(
	handle syscall.Handle,
	c *hcs.System,
	host *uvm.UtilityVM,
	tid string,
	eid string) {

	//	if host == nil {
	if handle == 0 {
		return
	}

	logrus.WithFields(logrus.Fields{
		"tid": tid,
		"eid": eid,
	}).Debug("Re-enabling VHD flushing")

	if err := setVhdWriteCacheMode(handle, writeCacheModeCacheMetadata); err != nil {
		panic("JJH")
	}
	syscall.CloseHandle(handle)
	//	}

	// ALL TODO HERE including defer error handling etc, logging, xenon,.....
	// // Xenon Path. We try our best regardless on both of these but don't fail if we can't.
	// modifyRegistryFlushState(host, true)
	// modifyAttachmentFlushState(c, 0, scratch, false)

	// JJH BIG TODO: ^^ The attachment ID. Can only assume 0 for non-pod scenarios

}

// modifyRegistryFlushState sends a modify request to a container to enable
// or disable the registry flush state.
func modifyRegistryFlushState(c *hcs.System, enabled bool) error {
	type registryFlushSetting struct {
		Enabled bool `json:"Enabled"`
	}

	return c.Modify(hcsschema.ModifySettingRequest{
		ResourcePath: "Container/RegistryFlushState",
		RequestType:  "Update",
		Settings:     registryFlushSetting{Enabled: enabled},
	})
}

// modifyAttachmentFlushState sends a modify request to a utility VM to enable
// or disable flushing on a containers scratch disk. Need to pass in the ID
// on the SCSI controller, and the path of the file in the utility VM.
func modifyAttachmentFlushState(host *uvm.UtilityVM, attachmentID uint16, scratch string, ignoreFlushes bool) error {
	return host.Modify(hcsschema.ModifySettingRequest{
		ResourcePath: fmt.Sprintf("VirtualMachine/Devices/Scsi/0/Attachments/%d", attachmentID),
		RequestType:  "Update",
		Settings: hcsschema.Attachment{
			Type_:         "VirtualDisk",
			Path:          scratch,
			IgnoreFlushes: ignoreFlushes,
		},
	})
}
