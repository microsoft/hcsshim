package uvm

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/sirupsen/logrus"
)

const (
	lcowSCSILayerFmt = "/run/layers/S%d/%d"
)

var (
	ErrNoAvailableLocation      = fmt.Errorf("no available location")
	ErrNotAttached              = fmt.Errorf("not attached")
	ErrAlreadyAttached          = fmt.Errorf("already attached")
	ErrNoSCSIControllers        = fmt.Errorf("no SCSI controllers configured for this utility VM")
	ErrTooManyAttachments       = fmt.Errorf("too many SCSI attachments")
	ErrSCSILayerWCOWUnsupported = fmt.Errorf("SCSI attached layers are not supported for WCOW")
)

// Release frees the resources of the corresponding Scsi Mount
func (sm *SCSIMount) Release(ctx context.Context) error {
	if err := sm.vm.RemoveSCSI(ctx, sm.HostPath); err != nil {
		log.G(ctx).WithError(err).Warn("failed to remove scsi device")
		return err
	}
	return nil
}

// SCSIMount struct representing a SCSI mount point and the UVM
// it belongs to.
type SCSIMount struct {
	// Utility VM the scsi mount belongs to
	vm *UtilityVM
	// path is the host path to the vhd that is mounted.
	HostPath string
	// path for the uvm
	UVMPath string
	// scsi controller
	Controller int
	// scsi logical unit number
	LUN int32
	// While most VHDs attached to SCSI are scratch spaces, in the case of LCOW
	// when the size is over the size possible to attach to PMEM, we use SCSI for
	// read-only layers. As RO layers are shared, we perform ref-counting.
	isLayer  bool
	refCount uint32
}

func (sm *SCSIMount) logFormat() logrus.Fields {
	return logrus.Fields{
		"HostPath":   sm.HostPath,
		"UVMPath":    sm.UVMPath,
		"isLayer":    sm.isLayer,
		"refCount":   sm.refCount,
		"Controller": sm.Controller,
		"LUN":        sm.LUN,
	}
}

// allocateSCSI finds the next available slot on the
// SCSI controllers associated with a utility VM to use.
// Lock must be held when calling this function
func (uvm *UtilityVM) allocateSCSI(ctx context.Context, hostPath string, uvmPath string, isLayer bool) (*SCSIMount, error) {
	for controller, luns := range uvm.scsiLocations {
		for lun, sm := range luns {
			// If sm is nil, we have found an open slot so we allocate a new SCSIMount
			if sm == nil {
				uvm.scsiLocations[controller][lun] = &SCSIMount{
					vm:         uvm,
					HostPath:   hostPath,
					UVMPath:    uvmPath,
					isLayer:    isLayer,
					refCount:   1,
					Controller: controller,
					LUN:        int32(lun),
				}
				log.G(ctx).WithFields(uvm.scsiLocations[controller][lun].logFormat()).Debug("allocated SCSI mount")
				return uvm.scsiLocations[controller][lun], nil
			}
		}
	}
	return nil, ErrNoAvailableLocation
}

func (uvm *UtilityVM) deallocateSCSI(ctx context.Context, sm *SCSIMount) {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if sm != nil {
		log.G(ctx).WithFields(sm.logFormat()).Debug("removed SCSI location")
		uvm.scsiLocations[sm.Controller][sm.LUN] = nil
	}
}

// Lock must be held when calling this function.
func (uvm *UtilityVM) findSCSIAttachment(ctx context.Context, findThisHostPath string) (*SCSIMount, error) {
	for _, luns := range uvm.scsiLocations {
		for _, sm := range luns {
			if sm != nil && sm.HostPath == findThisHostPath {
				log.G(ctx).WithFields(sm.logFormat()).Debug("found SCSI location")
				return sm, nil
			}
		}
	}
	return nil, ErrNotAttached
}

// RemoveSCSI removes a SCSI disk from a utility VM.
func (uvm *UtilityVM) RemoveSCSI(ctx context.Context, hostPath string) error {
	uvm.m.Lock()
	defer uvm.m.Unlock()

	if uvm.scsiControllerCount == 0 {
		return ErrNoSCSIControllers
	}

	// Make sure it is actually attached
	sm, err := uvm.findSCSIAttachment(ctx, hostPath)
	if err != nil {
		return err
	}

	sm.refCount--
	if sm.refCount > 0 {
		return nil
	}

	scsiModification := &hcsschema.ModifySettingRequest{
		RequestType:  requesttype.Remove,
		ResourcePath: fmt.Sprintf(scsiResourceFormat, strconv.Itoa(sm.Controller), sm.LUN),
	}

	// Include the GuestRequest so that the GCS ejects the disk cleanly if the
	// disk was attached/mounted
	//
	// Note: We always send a guest eject even if there is no UVM path in lcow
	// so that we synchronize the guest state. This seems to always avoid SCSI
	// related errors if this index quickly reused by another container.
	if uvm.operatingSystem == "windows" && sm.UVMPath != "" {
		scsiModification.GuestRequest = guestrequest.GuestRequest{
			ResourceType: guestrequest.ResourceTypeMappedVirtualDisk,
			RequestType:  requesttype.Remove,
			Settings: guestrequest.WCOWMappedVirtualDisk{
				ContainerPath: sm.UVMPath,
				Lun:           sm.LUN,
			},
		}
	} else {
		scsiModification.GuestRequest = guestrequest.GuestRequest{
			ResourceType: guestrequest.ResourceTypeMappedVirtualDisk,
			RequestType:  requesttype.Remove,
			Settings: guestrequest.LCOWMappedVirtualDisk{
				MountPath:  sm.UVMPath, // May be blank in attach-only
				Lun:        uint8(sm.LUN),
				Controller: uint8(sm.Controller),
			},
		}
	}

	if err := uvm.modify(ctx, scsiModification); err != nil {
		return fmt.Errorf("failed to remove SCSI disk %s from container %s: %s", hostPath, uvm.id, err)
	}
	uvm.scsiLocations[sm.Controller][sm.LUN] = nil
	return nil
}

// AddSCSI adds a SCSI disk to a utility VM at the next available location. This
// function should be called for a RW/scratch layer or a passthrough vhd/vhdx.
// For read-only layers on LCOW as an alternate to PMEM for large layers, use
// AddSCSILayer instead.
//
// `hostPath` is required and must point to a vhd/vhdx path.
//
// `uvmPath` is optional.
//
// `readOnly` set to `true` if the vhd/vhdx should be attached read only.
func (uvm *UtilityVM) AddSCSI(ctx context.Context, hostPath string, uvmPath string, readOnly bool) (*SCSIMount, error) {
	return uvm.addSCSIActual(ctx, hostPath, uvmPath, "VirtualDisk", false, readOnly)
}

// AddSCSIPhysicalDisk attaches a physical disk from the host directly to the
// Utility VM at the next available location.
//
// `hostPath` is required and `likely` start's with `\\.\PHYSICALDRIVE`.
//
// `uvmPath` is optional if a guest mount is not requested.
//
// `readOnly` set to `true` if the physical disk should be attached read only.
func (uvm *UtilityVM) AddSCSIPhysicalDisk(ctx context.Context, hostPath, uvmPath string, readOnly bool) (*SCSIMount, error) {
	return uvm.addSCSIActual(ctx, hostPath, uvmPath, "PassThru", false, readOnly)
}

// AddSCSILayer adds a read-only layer disk to a utility VM at the next
// available location and returns the path in the UVM where the layer was
// mounted. This function is used by LCOW as an alternate to PMEM for large
// layers.
func (uvm *UtilityVM) AddSCSILayer(ctx context.Context, hostPath string) (*SCSIMount, error) {
	if uvm.operatingSystem == "windows" {
		return nil, ErrSCSILayerWCOWUnsupported
	}

	sm, err := uvm.addSCSIActual(ctx, hostPath, "", "VirtualDisk", true, true)
	if err != nil {
		return nil, err
	}

	if sm.UVMPath != "" {
		return sm, nil
	}
	sm.UVMPath = fmt.Sprintf(lcowSCSILayerFmt, sm.Controller, sm.LUN)
	return sm, nil
}

// addSCSIActual is the implementation behind the external functions AddSCSI and
// AddSCSILayer.
//
// We are in control of everything ourselves. Hence we have ref- counting and
// so-on tracking what SCSI locations are available or used.
//
// `hostPath` is required and may be a vhd/vhdx or physical disk path.
//
// `uvmPath` is optional, and `must` be empty for layers. If `!isLayer` and
// `uvmPath` is empty no guest modify will take place.
//
// `attachmentType` is required and `must` be `VirtualDisk` for vhd/vhdx
// attachments and `PassThru` for physical disk.
//
// `isLayer` indicates that this is a read-only (LCOW) layer VHD. This parameter
// `must not` be used for Windows.
//
// `readOnly` indicates the attachment should be added read only.
//
// Returns the controller ID (0..3) and LUN (0..63) where the disk is attached.
func (uvm *UtilityVM) addSCSIActual(ctx context.Context, hostPath, uvmPath, attachmentType string, isLayer, readOnly bool) (*SCSIMount, error) {
	if uvm.scsiControllerCount == 0 {
		return nil, ErrNoSCSIControllers
	}

	// Ensure the utility VM has access
	if !isLayer {
		if err := wclayer.GrantVmAccess(ctx, uvm.id, hostPath); err != nil {
			return nil, err
		}
	}

	// We must hold the lock throughout the lookup (findSCSIAttachment) until
	// after the possible allocation (allocateSCSI) has been completed to ensure
	// there isn't a race condition for it being attached by another thread between
	// these two operations. All failure paths between these two must release
	// the lock.
	uvm.m.Lock()
	if sm, err := uvm.findSCSIAttachment(ctx, hostPath); err == nil {
		// SCSI disk is already attached, Increment the refcount
		sm.refCount++
		uvm.m.Unlock()
		return sm, nil
	}

	// At this point, we know it's not attached, regardless of whether it's a
	// ref-counted layer VHD, or not.
	sm, err := uvm.allocateSCSI(ctx, hostPath, uvmPath, isLayer)
	if err != nil {
		uvm.m.Unlock()
		return nil, err
	}
	defer func() {
		if err != nil {
			uvm.deallocateSCSI(ctx, sm)
		}
	}()

	// Auto-generate the UVM path for LCOW layers
	if isLayer {
		uvmPath = fmt.Sprintf(lcowSCSILayerFmt, sm.Controller, sm.LUN)
	}

	// See comment higher up. Now safe to release the lock.
	uvm.m.Unlock()

	// Note: Can remove this check post-RS5 if multiple controllers are supported
	if sm.Controller > 0 {
		return nil, ErrTooManyAttachments
	}

	SCSIModification := &hcsschema.ModifySettingRequest{
		RequestType: requesttype.Add,
		Settings: hcsschema.Attachment{
			Path:     hostPath,
			Type_:    attachmentType,
			ReadOnly: readOnly,
		},
		ResourcePath: fmt.Sprintf(scsiResourceFormat, strconv.Itoa(sm.Controller), sm.LUN),
	}

	if uvmPath != "" {
		guestReq := guestrequest.GuestRequest{
			ResourceType: guestrequest.ResourceTypeMappedVirtualDisk,
			RequestType:  requesttype.Add,
		}

		if uvm.operatingSystem == "windows" {
			guestReq.Settings = guestrequest.WCOWMappedVirtualDisk{
				ContainerPath: uvmPath,
				Lun:           sm.LUN,
			}
		} else {
			guestReq.Settings = guestrequest.LCOWMappedVirtualDisk{
				MountPath:  uvmPath,
				Lun:        uint8(sm.LUN),
				Controller: uint8(sm.Controller),
				ReadOnly:   readOnly,
			}
		}
		SCSIModification.GuestRequest = guestReq
	}

	if err := uvm.modify(ctx, SCSIModification); err != nil {
		return nil, fmt.Errorf("uvm::AddSCSI: failed to modify utility VM configuration: %s", err)
	}
	return sm, nil
}

// GetScsiUvmPath returns the guest mounted path of a SCSI drive.
//
// If `hostPath` is not mounted returns `ErrNotAttached`.
func (uvm *UtilityVM) GetScsiUvmPath(ctx context.Context, hostPath string) (string, error) {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	sm, err := uvm.findSCSIAttachment(ctx, hostPath)
	if err != nil {
		return "", err
	}
	return sm.UVMPath, err
}
