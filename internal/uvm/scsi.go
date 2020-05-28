package uvm

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Microsoft/go-winio/pkg/security"
	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// VMAccessType is used to determine the various types of access we can
// grant for a given file.
type VMAccessType int

const (
	// `VMAccessTypeNoop` indicates no additional access should be given. Note
	// this should be used for layers and gpu vhd where we have given VM group
	// access outside of the shim (containerd for layers, package installation
	// for gpu vhd).
	VMAccessTypeNoop VMAccessType = iota
	// `VMAccessTypeGroup` indicates we should give access to a file for the VM group sid
	VMAccessTypeGroup
	// `VMAccessTypeIndividual` indicates we should give additional access to a file for
	// the running VM only
	VMAccessTypeIndividual
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

// If a SCSI mount is read only then we should simply add it to the uvm config. But if it
// is a scratch layer (i.e writeable mount) then we should make a copy of it.
func (sm *SCSIMount) Clone(ctx context.Context, vm *UtilityVM, cd *CloneData) (interface{}, error) {
	var (
		dstVhdPath string = sm.HostPath
		err        error
		dir        string
	)
	conStr := fmt.Sprintf("%d", sm.Controller)
	lunStr := fmt.Sprintf("%d", sm.LUN)
	if !sm.IsReadOnly {
		// Copy this scsi disk
		// TODO(ambarve): This can be a SCSI mount that belongs to some container
		// which is being automatically cloned here as a part of UVM cloning
		// process. We will receive a request for creation of this container
		// later on which will specify the storage path for this container.
		// However, that storage location is not available now so we just use
		// the storage of the uvm instead. Find a better way for handling this.

		// For the scratch VHD of the VM clone it in the scratch folder
		if sm.Controller == 0 && sm.LUN == 0 {
			dir = cd.scratchFolder
		} else {
			dir, err = ioutil.TempDir(cd.scratchFolder, fmt.Sprintf("clone-mount-%d-%d", sm.Controller, sm.LUN))
			if err != nil {
				return nil, fmt.Errorf("Error while creating directory for scsi mounts of clone vm: %s", err)
			}
		}

		// copy the VHDX
		dstVhdPath = filepath.Join(dir, filepath.Base(sm.HostPath))
		log.G(ctx).Debug(fmt.Sprintf("cloning scsi mount with hostPath: %s, Controller: %d, LUN: %d to: %s", sm.HostPath, sm.Controller, sm.LUN, dstVhdPath))

		if err = copyfile.CopyFile(ctx, sm.HostPath, dstVhdPath, true); err != nil {
			return nil, err
		}

		if err = wclayer.GrantVmAccess(ctx, cd.UVMID, dstVhdPath); err != nil {
			os.Remove(dstVhdPath)
			return nil, err
		}
	}

	cd.doc.VirtualMachine.Devices.Scsi[conStr].Attachments[lunStr] = hcsschema.Attachment{
		Path: dstVhdPath,
		// TODO(ambarve) Get the correct type here
		Type_: "VirtualDisk",
	}

	clonedScsiMount := SCSIMount{
		vm:         vm,
		HostPath:   dstVhdPath,
		UVMPath:    sm.UVMPath,
		Controller: sm.Controller,
		LUN:        sm.LUN,
		refCount:   1,
		IsReadOnly: sm.IsReadOnly,
	}

	vm.scsiLocations[sm.Controller][sm.LUN] = &clonedScsiMount

	return &clonedScsiMount, nil
}

var _ = (Cloneable)(&SCSIMount{})

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
	// specifies if this is a readonly layer
	IsReadOnly bool
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

// allocateSCSISlot finds the next available slot on the
// SCSI controllers associated with a utility VM to use.
// Lock must be held when calling this function
func (uvm *UtilityVM) allocateSCSISlot(ctx context.Context, hostPath string, uvmPath string) (*SCSIMount, error) {
	for controller, luns := range uvm.scsiLocations {
		for lun, sm := range luns {
			// If sm is nil, we have found an open slot so we allocate a new SCSIMount
			if sm == nil {
				uvm.scsiLocations[controller][lun] = &SCSIMount{
					vm:         uvm,
					HostPath:   hostPath,
					UVMPath:    uvmPath,
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

func (uvm *UtilityVM) deallocateSCSIMount(ctx context.Context, sm *SCSIMount) {
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
// function should be called for a adding a scratch layer, a read-only layer as an
// alternative to VPMEM, or for other VHD mounts.
//
// `hostPath` is required and must point to a vhd/vhdx path.
//
// `uvmPath` is optional. If not provided, no guest request will be made
//
// `readOnly` set to `true` if the vhd/vhdx should be attached read only.
//
// `vmAccess` indicates what access to grant the vm for the hostpath
func (uvm *UtilityVM) AddSCSI(ctx context.Context, hostPath string, uvmPath string, readOnly bool, vmAccess VMAccessType) (*SCSIMount, error) {
	return uvm.addSCSIActual(ctx, hostPath, uvmPath, "VirtualDisk", readOnly, vmAccess)
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
	return uvm.addSCSIActual(ctx, hostPath, uvmPath, "PassThru", readOnly, VMAccessTypeIndividual)
}

// addSCSIActual is the implementation behind the external functions AddSCSI and
// AddSCSIPhysicalDisk.
//
// We are in control of everything ourselves. Hence we have ref- counting and
// so-on tracking what SCSI locations are available or used.
//
// `attachmentType` is required and `must` be `VirtualDisk` for vhd/vhdx
// attachments and `PassThru` for physical disk.
//
// `readOnly` indicates the attachment should be added read only.
//
// `vmAccess` indicates what access to grant the vm for the hostpath
//
// Returns result from calling modify with the given scsi mount
func (uvm *UtilityVM) addSCSIActual(ctx context.Context, hostPath, uvmPath, attachmentType string, readOnly bool, vmAccess VMAccessType) (sm *SCSIMount, err error) {
	sm, existed, err := uvm.allocateSCSIMount(ctx, hostPath, uvmPath, vmAccess)
	if err != nil {
		return nil, err
	}
	sm.IsReadOnly = readOnly

	defer func() {
		if err != nil {
			uvm.deallocateSCSIMount(ctx, sm)
		}
	}()

	if existed {
		return sm, nil
	}

	if uvm.scsiControllerCount == 0 {
		return nil, ErrNoSCSIControllers
	}

	// Note: Can remove this check post-RS5 if multiple controllers are supported
	if sm.Controller > 0 {
		return nil, ErrTooManyAttachments
	}

	SCSIModification := &hcsschema.ModifySettingRequest{
		RequestType: requesttype.Add,
		Settings: hcsschema.Attachment{
			Path:     sm.HostPath,
			Type_:    attachmentType,
			ReadOnly: readOnly,
		},
		ResourcePath: fmt.Sprintf(scsiResourceFormat, strconv.Itoa(sm.Controller), sm.LUN),
	}

	if sm.UVMPath != "" {
		guestReq := guestrequest.GuestRequest{
			ResourceType: guestrequest.ResourceTypeMappedVirtualDisk,
			RequestType:  requesttype.Add,
		}

		if uvm.operatingSystem == "windows" {
			guestReq.Settings = guestrequest.WCOWMappedVirtualDisk{
				ContainerPath: sm.UVMPath,
				Lun:           sm.LUN,
			}
		} else {
			guestReq.Settings = guestrequest.LCOWMappedVirtualDisk{
				MountPath:  sm.UVMPath,
				Lun:        uint8(sm.LUN),
				Controller: uint8(sm.Controller),
				ReadOnly:   readOnly,
			}
		}
		SCSIModification.GuestRequest = guestReq
	}

	if err := uvm.modify(ctx, SCSIModification); err != nil {
		return nil, fmt.Errorf("failed to modify UVM with new SCSI mount: %s", err)
	}
	return sm, nil
}

// allocateSCSIMount grants vm access to hostpath and increments the ref count of an existing scsi
// device or allocates a new one if not already present.
// Returns the resulting *SCSIMount, a bool indicating if the scsi device was already present,
// and error if any.
func (uvm *UtilityVM) allocateSCSIMount(ctx context.Context, hostPath, uvmPath string, vmAccess VMAccessType) (*SCSIMount, bool, error) {
	// Ensure the utility VM has access
	err := uvm.grantAccess(ctx, hostPath, vmAccess)
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to grant VM access for SCSI mount")
	}
	// We must hold the lock throughout the lookup (findSCSIAttachment) until
	// after the possible allocation (allocateSCSISlot) has been completed to ensure
	// there isn't a race condition for it being attached by another thread between
	// these two operations.
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if sm, err := uvm.findSCSIAttachment(ctx, hostPath); err == nil {
		sm.refCount++
		return sm, true, nil
	}

	sm, err := uvm.allocateSCSISlot(ctx, hostPath, uvmPath)
	if err != nil {
		return nil, false, err
	}
	return sm, false, nil
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

// grantAccess helper function to grant access to a file for the vm or vm group
func (uvm *UtilityVM) grantAccess(ctx context.Context, hostPath string, vmAccess VMAccessType) error {
	switch vmAccess {
	case VMAccessTypeGroup:
		log.G(ctx).WithField("path", hostPath).Debug("granting vm group access")
		return security.GrantVmGroupAccess(hostPath)
	case VMAccessTypeIndividual:
		return wclayer.GrantVmAccess(ctx, uvm.id, hostPath)
	}
	return nil
}

// GetScsiLocationInfo returns the controller number and LUN for the SCSI disk that is
// referenced by hostPath
func (uvm *UtilityVM) GetScsiLocationInfo(ctx context.Context, hostPath string) (int, int32, error) {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	sm, err := uvm.findSCSIAttachment(ctx, hostPath)
	if err != nil {
		return -1, -1, err
	}
	return sm.Controller, sm.LUN, nil
}

// Scratch VHD of the UVM is always attached at SCSI 0:0
func (uvm *UtilityVM) GetScratchVHDPath() (string, error) {
	if uvm.scsiLocations[0][0] != nil {
		return uvm.scsiLocations[0][0].HostPath, nil
	}
	return "", fmt.Errorf("No scratch VHD found for the uvm\n")
}
