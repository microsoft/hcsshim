//go:build windows

package uvm

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/security"
	"github.com/Microsoft/hcsshim/internal/wclayer"
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

const scsiCurrentSerialVersionID = 2

var (
	ErrNoAvailableLocation         = fmt.Errorf("no available location")
	ErrNotAttached                 = fmt.Errorf("not attached")
	ErrAlreadyAttached             = fmt.Errorf("already attached")
	ErrNoSCSIControllers           = fmt.Errorf("no SCSI controllers configured for this utility VM")
	ErrTooManyAttachments          = fmt.Errorf("too many SCSI attachments")
	ErrSCSILayerWCOWUnsupported    = fmt.Errorf("SCSI attached layers are not supported for WCOW")
	ErrAttachmentHasMultipleMounts = fmt.Errorf("SCSI attachment has multipl mounts")
)

func (gm *SCSIMount) Release(ctx context.Context) error {
	if err := gm.vm.RemoveSCSIMount(ctx, gm.HostPath, gm.UVMPath); err != nil {
		return fmt.Errorf("failed to remove SCSI device: %s", err)
	}
	return nil
}

// SCSIAttachment struct representing a SCSI mount point and the UVM
// it belongs to.
type SCSIAttachment struct {
	// path is the host path to the vhd that is mounted.
	HostPath string
	mounts   map[string]*SCSIMount
	// scsi controller
	Controller int
	// scsi logical unit number
	LUN int32
	// While most VHDs attached to SCSI are scratch spaces, in the case of LCOW
	// when the size is over the size possible to attach to PMEM, we use SCSI for
	// read-only layers.
	isLayer bool
	// ref count the attachment so we know when to remove it
	refCount uint32
	// specifies if this is an encrypted VHD
	encrypted bool
	// specifies if this is a readonly layer
	readOnly bool
	// "VirtualDisk" or "PassThru" or "ExtensibleVirtualDisk" disk attachment type.
	attachmentType string
	// If attachmentType is "ExtensibleVirtualDisk" then extensibleVirtualDiskType should
	// specify the type of it (for e.g "space" for storage spaces). Otherwise this should be
	// empty.
	extensibleVirtualDiskType string
	// serialization ID
	serialVersionID uint32
	// Make sure that serialVersionID is always the last field and its value is
	// incremented every time this structure is updated
}

// SCSIMount
// all UVMPaths must be unique in the uvm
type SCSIMount struct {
	HostPath string // used for finding the scsi attachment
	vm       *UtilityVM

	UVMPath   string
	partition uint8

	refCount uint32

	// A channel to wait on while mount of this SCSI disk is in progress.
	waitCh chan struct{}
	// The error field that is set if the mounting of this disk fails. Any other waiters on waitCh
	// can use this waitErr after the channel is closed.
	waitErr error
}

func (gm *SCSIMount) logFormat() logrus.Fields {
	return logrus.Fields{
		"HostPath":  gm.HostPath,
		"UVMPath":   gm.UVMPath,
		"Partition": gm.partition,
		"refCount":  gm.refCount,
	}
}

// addSCSIRequest is an internal struct used to hold all the parameters that are sent to
// the addSCSIActual method.
type addSCSIRequest struct {
	// host path to the disk that should be added as a SCSI disk.
	hostPath string
	// the path inside the uvm at which this disk should show up. Can be empty.
	uvmPath   string
	partition uint8
	isLayer   bool
	// attachmentType is required and `must` be `VirtualDisk` for vhd/vhdx
	// attachments, `PassThru` for physical disk and `ExtensibleVirtualDisk` for
	// Extensible virtual disks.
	attachmentType string
	// indicates if the VHD is encrypted
	encrypted bool
	// indicates if the attachment should be added read only.
	readOnly bool
	// guestOptions is a slice that contains optional information to pass to the guest
	// service.
	guestOptions []string
	// indicates what access to grant the vm for the hostpath. Only required for
	// `VirtualDisk` and `PassThru` disk types.
	vmAccess VMAccessType
	// `evdType` indicates the type of the extensible virtual disk if `attachmentType`
	// is "ExtensibleVirtualDisk" should be empty otherwise.
	evdType string
}

// RefCount returns the current refcount for the SCSI mount.
func (sm *SCSIMount) RefCount() uint32 {
	return sm.refCount
}

func newSCSIMount(
	uvm *UtilityVM,
	hostPath string,
	uvmPath string,
	partition uint8,
	refCount uint32,
) *SCSIMount {
	return &SCSIMount{
		vm:        uvm,
		HostPath:  hostPath,
		UVMPath:   uvmPath,
		partition: partition,
		refCount:  refCount,
		waitCh:    make(chan struct{}),
	}
}

func newSCSIAttachment(
	hostPath string,
	attachmentType string,
	evdType string,
	refCount uint32,
	controller int,
	lun int32,
	readOnly bool,
	encrypted bool,
	isLayer bool,
) *SCSIAttachment {
	return &SCSIAttachment{
		HostPath:                  hostPath,
		mounts:                    make(map[string]*SCSIMount),
		refCount:                  refCount,
		Controller:                controller,
		LUN:                       int32(lun),
		isLayer:                   isLayer,
		encrypted:                 encrypted,
		readOnly:                  readOnly,
		attachmentType:            attachmentType,
		extensibleVirtualDiskType: evdType,
		serialVersionID:           scsiCurrentSerialVersionID,
	}
}

func (sm *SCSIAttachment) logFormat() logrus.Fields {
	return logrus.Fields{
		"HostPath":                  sm.HostPath,
		"isLayer":                   sm.isLayer,
		"refCount":                  sm.refCount,
		"Controller":                sm.Controller,
		"LUN":                       sm.LUN,
		"ExtensibleVirtualDiskType": sm.extensibleVirtualDiskType,
		"SerialVersionID":           sm.serialVersionID,
	}
}

// AddSCSI adds a SCSI disk to a utility VM at the next available location. This
// function should be called for adding a scratch layer, a read-only layer as an
// alternative to VPMEM, or for other VHD mounts.
//
// `hostPath` is required and must point to a vhd/vhdx path.
//
// `uvmPath` is optional. If not provided, no guest request will be made
//
// `readOnly` set to `true` if the vhd/vhdx should be attached read only.
//
// `encrypted` set to `true` if the vhd/vhdx should be attached in encrypted mode.
// The device will be formatted, so this option must be used only when creating
// scratch vhd/vhdx.
//
// `guestOptions` is a slice that contains optional information to pass
// to the guest service
//
// `vmAccess` indicates what access to grant the vm for the hostpath
func (uvm *UtilityVM) AddSCSI(
	ctx context.Context,
	hostPath string,
	uvmPath string,
	readOnly bool,
	encrypted bool,
	guestOptions []string,
	vmAccess VMAccessType,
) (*SCSIMount, error) {
	addReq := &addSCSIRequest{
		hostPath:       hostPath,
		uvmPath:        uvmPath,
		attachmentType: "VirtualDisk",
		readOnly:       readOnly,
		encrypted:      encrypted,
		guestOptions:   guestOptions,
		vmAccess:       vmAccess,
	}
	return uvm.addSCSIActual(ctx, addReq)
}

// AddSCSIPhysicalDisk attaches a physical disk from the host directly to the
// Utility VM at the next available location.
//
// `hostPath` is required and `likely` start's with `\\.\PHYSICALDRIVE`.
//
// `uvmPath` is optional if a guest mount is not requested.
//
// `readOnly` set to `true` if the physical disk should be attached read only.
//
// `guestOptions` is a slice that contains optional information to pass
// to the guest service
func (uvm *UtilityVM) AddSCSIPhysicalDisk(ctx context.Context, hostPath, uvmPath string, readOnly bool, guestOptions []string) (*SCSIMount, error) {
	addReq := &addSCSIRequest{
		hostPath:       hostPath,
		uvmPath:        uvmPath,
		attachmentType: "PassThru",
		readOnly:       readOnly,
		guestOptions:   guestOptions,
		vmAccess:       VMAccessTypeIndividual,
	}
	return uvm.addSCSIActual(ctx, addReq)
}

// AddSCSIExtensibleVirtualDisk adds an extensible virtual disk as a SCSI mount
// to the utility VM at the next available location. All such disks which are not actual virtual disks
// but provide the same SCSI interface are added to the UVM as Extensible Virtual disks.
//
// `hostPath` is required. Depending on the type of the extensible virtual disk the format of `hostPath` can
// be different.
// For example, in case of storage spaces the host path must be in the
// `evd://space/{storage_pool_unique_ID}{virtual_disk_unique_ID}` format.
//
// `uvmPath` must be provided in order to be able to use this disk in a container.
//
// `readOnly` set to `true` if the virtual disk should be attached read only.
//
// `vmAccess` indicates what access to grant the vm for the hostpath
func (uvm *UtilityVM) AddSCSIExtensibleVirtualDisk(ctx context.Context, hostPath, uvmPath string, readOnly bool) (*SCSIMount, error) {
	if uvmPath == "" {
		return nil, errors.New("uvmPath can not be empty for extensible virtual disk")
	}
	evdType, mountPath, err := ParseExtensibleVirtualDiskPath(hostPath)
	if err != nil {
		return nil, err
	}
	addReq := &addSCSIRequest{
		hostPath:       mountPath,
		uvmPath:        uvmPath,
		attachmentType: "ExtensibleVirtualDisk",
		readOnly:       readOnly,
		guestOptions:   []string{},
		vmAccess:       VMAccessTypeIndividual,
		evdType:        evdType,
	}
	return uvm.addSCSIActual(ctx, addReq)
}

// allocateSCSIMount grants vm access to hostpath and increments the ref count of an existing scsi
// device or allocates a new one if not already present.
// Returns the resulting *SCSIMount, a bool indicating if the scsi device was already present,
// and error if any.
func (uvm *UtilityVM) allocateSCSIAttachment(
	ctx context.Context,
	readOnly bool,
	encrypted bool,
	hostPath string,
	attachmentType string,
	evdType string,
	vmAccess VMAccessType,
	isLayer bool,
) (*SCSIAttachment, bool, error) {
	if attachmentType != "ExtensibleVirtualDisk" {
		// Ensure the utility VM has access
		err := grantAccess(ctx, uvm.id, hostPath, vmAccess)
		if err != nil {
			return nil, false, errors.Wrapf(err, "failed to grant VM access for SCSI mount")
		}
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

	controller, lun, err := uvm.allocateSCSISlot(ctx)
	if err != nil {
		return nil, false, err
	}

	uvm.scsiLocations[controller][lun] = newSCSIAttachment(
		hostPath,
		attachmentType,
		evdType,
		1,
		controller,
		int32(lun),
		readOnly,
		encrypted,
		isLayer,
	)

	log.G(ctx).WithFields(uvm.scsiLocations[controller][lun].logFormat()).Debug("allocated SCSI attachment")

	return uvm.scsiLocations[controller][lun], false, nil
}

// GetFirstSCSIMountUVMPath returns the guest mounted path of a SCSI drive.
//
// If `hostPath` is not mounted returns `ErrNotAttached`.
func (uvm *UtilityVM) GetFirstSCSIMountUVMPath(ctx context.Context, hostPath string) (string, error) {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	// make sure it's attached
	sm, err := uvm.findSCSIAttachment(ctx, hostPath)
	if err != nil {
		return "", err
	}

	gm, err := sm.findFirstSCSIMount(ctx)
	if err != nil {
		return "", err
	}
	return gm.UVMPath, nil
}

// ScratchEncryptionEnabled is a getter for `uvm.encryptScratch`.
//
// Returns true if the scratch disks should be encrypted, false otherwise.
func (uvm *UtilityVM) ScratchEncryptionEnabled() bool {
	return uvm.encryptScratch
}

// grantAccess helper function to grant access to a file for the vm or vm group
func grantAccess(ctx context.Context, uvmID string, hostPath string, vmAccess VMAccessType) error {
	switch vmAccess {
	case VMAccessTypeGroup:
		log.G(ctx).WithField("path", hostPath).Debug("granting vm group access")
		return security.GrantVmGroupAccess(hostPath)
	case VMAccessTypeIndividual:
		return wclayer.GrantVmAccess(ctx, uvmID, hostPath)
	}
	return nil
}

func (sm *SCSIAttachment) GetSerialVersionID() uint32 {
	return scsiCurrentSerialVersionID
}

// ParseExtensibleVirtualDiskPath parses the evd path provided in the config.
// extensible virtual disk path has format "evd://<evdType>/<evd-mount-path>"
// this function parses that and returns the `evdType` and `evd-mount-path`.
func ParseExtensibleVirtualDiskPath(hostPath string) (evdType, mountPath string, err error) {
	trimmedPath := strings.TrimPrefix(hostPath, "evd://")
	separatorIndex := strings.Index(trimmedPath, "/")
	if separatorIndex <= 0 {
		return "", "", errors.Errorf("invalid extensible vhd path: %s", hostPath)
	}
	return trimmedPath[:separatorIndex], trimmedPath[separatorIndex+1:], nil
}

// allocateSCSISlot finds the next available slot on the
// SCSI controllers associated with a utility VM to use.
// Lock must be held when calling this function
func (uvm *UtilityVM) allocateSCSISlot(ctx context.Context) (int, int, error) {
	for controller := 0; controller < int(uvm.scsiControllerCount); controller++ {
		for lun, sm := range uvm.scsiLocations[controller] {
			// If sm is nil, we have found an open slot so we allocate a new SCSIMount
			if sm == nil {
				return controller, lun, nil
			}
		}
	}
	return -1, -1, ErrNoAvailableLocation
}

func (uvm *UtilityVM) deallocateSCSIAttachment(ctx context.Context, sm *SCSIAttachment) {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if sm != nil {
		log.G(ctx).WithFields(sm.logFormat()).Debug("removed SCSI location")
		uvm.scsiLocations[sm.Controller][sm.LUN] = nil
	}
}

// Lock must be held when calling this function.
func (uvm *UtilityVM) findSCSIAttachment(ctx context.Context, findThisHostPath string) (*SCSIAttachment, error) {
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

func (sm *SCSIAttachment) findSCSIMount(ctx context.Context, findThisUVMPath string) (*SCSIMount, error) {
	for _, gm := range sm.mounts {
		if gm != nil && gm.UVMPath == findThisUVMPath {
			// TODO katiewasnothere: check if this still works when we don't have
			// a uvm path
			log.G(ctx).WithFields(gm.logFormat()).Debug("found SCSI mount")
			return gm, nil
		}
	}
	return nil, ErrNotAttached
}

// this is similar to findSCSIMount except that it finds the first mount with the host path
// and does not account for uvm path
func (sm *SCSIAttachment) findFirstSCSIMount(ctx context.Context) (*SCSIMount, error) {
	for _, gm := range sm.mounts {
		log.G(ctx).WithFields(gm.logFormat()).Debug("found SCSI mount")
		return gm, nil
	}
	return nil, ErrNotAttached
}

// allocateSCSIMount grants vm access to hostpath and increments the ref count of an existing scsi
// device or allocates a new one if not already present.
// Returns the resulting *SCSIMount, a bool indicating if the scsi device was already present,
// and error if any.
func (uvm *UtilityVM) allocateSCSIMount(
	ctx context.Context,
	sm *SCSIAttachment,
	hostPath string,
	uvmPath string,
	partition uint8,
	allowMultipleGuestMounts bool,
) (_ *SCSIMount, _ bool, err error) {
	// TODO katiewasnothere: do we need to hold the lock here?
	uvm.m.Lock()
	defer uvm.m.Unlock()
	var gm *SCSIMount
	if allowMultipleGuestMounts {
		// if we allow multiple scsi mounts for a given hostPath, then we should
		// search by both the hostPath and uvmPath
		if gm, err = sm.findSCSIMount(ctx, uvmPath); err == nil {
			gm.refCount++
			return gm, true, nil
		}
	} else {
		// else just check if a scsi mount exists only by the hostPath
		if gm, err = sm.findFirstSCSIMount(ctx); err == nil {
			gm.refCount++
			return gm, true, nil
		}
	}

	scsiMount := newSCSIMount(
		uvm,
		hostPath,
		uvmPath,
		partition,
		1,
	)
	sm.mounts[uvmPath] = scsiMount

	log.G(ctx).WithFields(scsiMount.logFormat()).Debug("allocated SCSI mount")

	return scsiMount, false, nil
}

func (uvm *UtilityVM) deallocateSCSIMount(ctx context.Context, sm *SCSIAttachment, gm *SCSIMount) {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if sm != nil && gm != nil {
		log.G(ctx).WithFields(gm.logFormat()).Debug("removed SCSI mount")
		sm.mounts[gm.UVMPath] = nil
	}
}

// addSCSIActual is the implementation behind the external functions AddSCSI,
// AddSCSIPhysicalDisk, AddSCSIExtensibleVirtualDisk.
//
// We are in control of everything ourselves. Hence we have ref- counting and
// so-on tracking what SCSI locations are available or used.
//
// Returns result from calling modify with the given scsi mount
func (uvm *UtilityVM) addSCSIActual(ctx context.Context, addReq *addSCSIRequest) (_ *SCSIMount, err error) {
	sm, attachmentExisted, err := uvm.allocateSCSIAttachment(
		ctx,
		addReq.readOnly,
		addReq.encrypted,
		addReq.hostPath,
		addReq.attachmentType,
		addReq.evdType,
		addReq.vmAccess,
		addReq.isLayer,
	)
	if err != nil {
		return nil, err
	}

	// continue even if the attachment already exists
	// TODO TODO TODO katiewasnothere: only allocate new SCSIMount if it either
	// doesn't exist OR this is a layer
	allowMultipleGuestMounts := addReq.isLayer && uvm.operatingSystem != "windows"
	gm, mountExisted, err := uvm.allocateSCSIMount(
		ctx,
		sm,
		addReq.hostPath,
		addReq.uvmPath,
		addReq.partition,
		allowMultipleGuestMounts,
	)
	if err != nil {
		return nil, err
	}

	if mountExisted {
		// another mount request might be in progress, wait for it to finish and if that operation
		// fails return that error.
		<-gm.waitCh
		if gm.waitErr != nil {
			return nil, gm.waitErr
		}
		return gm, nil
	}

	// This is the first goroutine to add this disk, close the waitCh after we are done.
	defer func() {
		if err != nil {
			// TODO katiewasnothere: only deallocate scsi attachment if it didn't already exist
			if !attachmentExisted {
				uvm.deallocateSCSIAttachment(ctx, sm)
			}
			uvm.deallocateSCSIMount(ctx, sm, gm)
		}

		// error must be set _before_ the channel is closed.
		gm.waitErr = err
		close(gm.waitCh)
	}()

	SCSIModification := &hcsschema.ModifySettingRequest{}

	if !attachmentExisted {
		// only add the attachment request if the attachment hadn't already existed
		SCSIModification = &hcsschema.ModifySettingRequest{
			RequestType: guestrequest.RequestTypeAdd,
			Settings: hcsschema.Attachment{
				Path:                      sm.HostPath,
				Type_:                     addReq.attachmentType,
				ReadOnly:                  addReq.readOnly,
				ExtensibleVirtualDiskType: addReq.evdType,
			},
			ResourcePath: fmt.Sprintf(resourcepaths.SCSIResourceFormat, guestrequest.ScsiControllerGuids[sm.Controller], sm.LUN),
		}
	}

	if gm.UVMPath != "" {
		guestReq := guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedVirtualDisk,
			RequestType:  guestrequest.RequestTypeAdd,
		}

		if uvm.operatingSystem == "windows" {
			guestReq.Settings = guestresource.WCOWMappedVirtualDisk{
				ContainerPath: gm.UVMPath,
				Lun:           sm.LUN,
			}
		} else {
			var verity *guestresource.DeviceVerityInfo
			if v, iErr := readVeritySuperBlock(ctx, gm.HostPath); iErr != nil {
				log.G(ctx).WithError(iErr).WithField("hostPath", gm.HostPath).Debug("unable to read dm-verity information from VHD")
			} else {
				if v != nil {
					log.G(ctx).WithFields(logrus.Fields{
						"hostPath":   gm.HostPath,
						"rootDigest": v.RootDigest,
					}).Debug("adding SCSI with dm-verity")
				}
				verity = v
			}

			guestReq.Settings = guestresource.LCOWMappedVirtualDisk{
				MountPath:  gm.UVMPath,
				Lun:        uint8(sm.LUN),
				Controller: uint8(sm.Controller),
				ReadOnly:   addReq.readOnly,
				Encrypted:  addReq.encrypted,
				Options:    addReq.guestOptions,
				VerityInfo: verity,
			}
		}
		SCSIModification.GuestRequest = guestReq
	}

	if err := uvm.modify(ctx, SCSIModification); err != nil {
		return nil, fmt.Errorf("failed to modify UVM with new SCSI mount: %s", err)
	}
	return gm, nil
}

// RemoveSCSI removes a SCSI disk from a utility VM.
func (uvm *UtilityVM) RemoveSCSIMount(ctx context.Context, hostPath, uvmPath string) error {
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

	// get the mount
	gm, err := sm.findSCSIMount(ctx, uvmPath)
	if err != nil {
		return err
	}

	sm.refCount--
	gm.refCount--
	if gm.refCount > 0 {
		return nil
	}

	removeAttachment := (sm.refCount <= 0)
	scsiModification := &hcsschema.ModifySettingRequest{}

	if removeAttachment {
		scsiModification = &hcsschema.ModifySettingRequest{
			RequestType:  guestrequest.RequestTypeRemove,
			ResourcePath: fmt.Sprintf(resourcepaths.SCSIResourceFormat, guestrequest.ScsiControllerGuids[sm.Controller], sm.LUN),
		}
	}

	var verity *guestresource.DeviceVerityInfo
	if v, iErr := readVeritySuperBlock(ctx, hostPath); iErr != nil {
		log.G(ctx).WithError(iErr).WithField("hostPath", sm.HostPath).Debug("unable to read dm-verity information from VHD")
	} else {
		if v != nil {
			log.G(ctx).WithFields(logrus.Fields{
				"hostPath":   hostPath,
				"rootDigest": v.RootDigest,
			}).Debug("removing SCSI with dm-verity")
		}
		verity = v
	}

	// Include the GuestRequest so that the GCS ejects the disk cleanly if the
	// disk was attached/mounted
	//
	// Note: We always send a guest eject even if there is no UVM path in lcow
	// so that we synchronize the guest state. This seems to always avoid SCSI
	// related errors if this index quickly reused by another container.
	if uvm.operatingSystem == "windows" && gm.UVMPath != "" {
		scsiModification.GuestRequest = guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedVirtualDisk,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings: guestresource.WCOWMappedVirtualDisk{
				ContainerPath: gm.UVMPath,
				Lun:           sm.LUN,
			},
		}
	} else {
		scsiModification.GuestRequest = guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedVirtualDisk,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings: guestresource.LCOWMappedVirtualDisk{
				MountPath:  gm.UVMPath, // May be blank in attach-only
				Lun:        uint8(sm.LUN),
				Controller: uint8(sm.Controller),
				VerityInfo: verity,
			},
		}
	}

	if err := uvm.modify(ctx, scsiModification); err != nil {
		return fmt.Errorf("failed to remove SCSI disk %s from container %s: %s", hostPath, uvm.id, err)
	}

	if removeAttachment {
		uvm.scsiLocations[sm.Controller][sm.LUN] = nil
		log.G(ctx).WithFields(sm.logFormat()).Debug("removed SCSI attachment")
	}
	sm.mounts[gm.UVMPath] = nil
	log.G(ctx).WithFields(gm.logFormat()).Debug("removed SCSI mount")

	return nil
}

// AddSCSILayer  adds a SCSI disk to a utility VM at the next available location. This
// function should be called for adding a scratch layer, a read-only layer as an
// alternative to VPMEM, or for other VHD mounts.
//
// `hostPath` is required and must point to a vhd/vhdx path.
//
// `uvmPath` is optional. If not provided, no guest request will be made
//
// `readOnly` set to `true` if the vhd/vhdx should be attached read only.
//
// `encrypted` set to `true` if the vhd/vhdx should be attached in encrypted mode.
// The device will be formatted, so this option must be used only when creating
// scratch vhd/vhdx.
//
// `guestOptions` is a slice that contains optional information to pass
// to the guest service
//
// `vmAccess` indicates what access to grant the vm for the hostpath
func (uvm *UtilityVM) AddSCSILayer(
	ctx context.Context,
	hostPath string,
	uvmPath string,
	partition uint8,
	readOnly bool,
	encrypted bool,
	guestOptions []string,
	vmAccess VMAccessType,
) (*SCSIMount, error) {
	addReq := &addSCSIRequest{
		hostPath:       hostPath,
		uvmPath:        uvmPath,
		partition:      partition,
		attachmentType: "VirtualDisk",
		isLayer:        true,
		readOnly:       readOnly,
		encrypted:      encrypted,
		guestOptions:   guestOptions,
		vmAccess:       vmAccess,
	}
	return uvm.addSCSIActual(ctx, addReq)
}

var _ = (Cloneable)(&SCSIAttachment{})

// GobEncode serializes the SCSIMount struct
func (sm *SCSIAttachment) GobEncode() ([]byte, error) {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	errMsgFmt := "failed to encode SCSIMount: %s"
	// encode only the fields that can be safely deserialized.
	if err := encoder.Encode(sm.serialVersionID); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sm.HostPath); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sm.mounts); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sm.Controller); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sm.LUN); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sm.readOnly); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sm.attachmentType); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sm.extensibleVirtualDiskType); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sm.isLayer); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	return buf.Bytes(), nil
}

// GobDecode deserializes the SCSIMount struct into the struct on which this is called
// (i.e the sm pointer)
func (sm *SCSIAttachment) GobDecode(data []byte) error {
	buf := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buf)
	errMsgFmt := "failed to decode SCSIMount: %s"
	// fields should be decoded in the same order in which they were encoded.
	if err := decoder.Decode(&sm.serialVersionID); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if sm.serialVersionID != scsiCurrentSerialVersionID {
		return fmt.Errorf("serialized version of SCSIMount: %d doesn't match with the current version: %d", sm.serialVersionID, scsiCurrentSerialVersionID)
	}
	if err := decoder.Decode(&sm.HostPath); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if err := decoder.Decode(&sm.mounts); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if err := decoder.Decode(&sm.Controller); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if err := decoder.Decode(&sm.LUN); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if err := decoder.Decode(&sm.readOnly); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if err := decoder.Decode(&sm.attachmentType); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if err := decoder.Decode(&sm.extensibleVirtualDiskType); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if err := decoder.Decode(&sm.isLayer); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	return nil
}

// Clone function creates a clone of the SCSIAttachment `sm` and adds the cloned SCSIAttachment to
// the uvm `vm`. If `sm` is read only then it is simply added to the `vm`. But if it is a
// writable mount(e.g a scratch layer) then a copy of it is made and that copy is added
// to the `vm`.
func (sm *SCSIAttachment) Clone(ctx context.Context, vm *UtilityVM, cd *cloneData) error {
	var (
		dstVhdPath string = sm.HostPath
		err        error
		dir        string
		conStr     string = guestrequest.ScsiControllerGuids[sm.Controller]
		lunStr     string = fmt.Sprintf("%d", sm.LUN)
	)

	if !sm.readOnly {
		// This is a writable SCSI mount. It must be either the
		// 1. scratch VHD of the UVM or
		// 2. scratch VHD of the container.
		// A user provided writable SCSI mount is not allowed on the template UVM
		// or container and so this SCSI mount has to be the scratch VHD of the
		// UVM or container.  The container inside this UVM will automatically be
		// cloned here when we are cloning the uvm itself. We will receive a
		// request for creation of this container later and that request will
		// specify the storage path for this container.  However, that storage
		// location is not available now so we just use the storage path of the
		// uvm instead.
		// TODO(ambarve): Find a better way for handling this. Problem with this
		// approach is that the scratch VHD of the container will not be
		// automatically cleaned after container exits. It will stay there as long
		// as the UVM keeps running.

		// For the scratch VHD of the VM (always attached at Controller:0, LUN:0)
		// clone it in the scratch folder
		dir = cd.scratchFolder
		if sm.Controller != 0 || sm.LUN != 0 {
			dir, err = os.MkdirTemp(cd.scratchFolder, fmt.Sprintf("clone-mount-%d-%d", sm.Controller, sm.LUN))
			if err != nil {
				return fmt.Errorf("error while creating directory for scsi mounts of clone vm: %s", err)
			}
		}

		// copy the VHDX
		dstVhdPath = filepath.Join(dir, filepath.Base(sm.HostPath))
		log.G(ctx).WithFields(logrus.Fields{
			"source hostPath":      sm.HostPath,
			"controller":           sm.Controller,
			"LUN":                  sm.LUN,
			"destination hostPath": dstVhdPath,
		}).Debug("Creating a clone of SCSI mount")

		if err = copyfile.CopyFile(ctx, sm.HostPath, dstVhdPath, true); err != nil {
			return err
		}

		if err = grantAccess(ctx, cd.uvmID, dstVhdPath, VMAccessTypeIndividual); err != nil {
			os.Remove(dstVhdPath)
			return err
		}
	}

	if cd.doc.VirtualMachine.Devices.Scsi == nil {
		cd.doc.VirtualMachine.Devices.Scsi = map[string]hcsschema.Scsi{}
	}

	if _, ok := cd.doc.VirtualMachine.Devices.Scsi[conStr]; !ok {
		cd.doc.VirtualMachine.Devices.Scsi[conStr] = hcsschema.Scsi{
			Attachments: map[string]hcsschema.Attachment{},
		}
	}

	cd.doc.VirtualMachine.Devices.Scsi[conStr].Attachments[lunStr] = hcsschema.Attachment{
		Path:  dstVhdPath,
		Type_: sm.attachmentType,
	}

	clonedSCSIAttachment := newSCSIAttachment(
		dstVhdPath,
		sm.attachmentType,
		sm.extensibleVirtualDiskType,
		1,
		sm.Controller,
		sm.LUN,
		sm.readOnly,
		sm.encrypted,
		sm.isLayer,
	)

	for _, gm := range sm.mounts {
		clonedSCSIMount := newSCSIMount(
			gm.vm,
			dstVhdPath,
			gm.UVMPath,
			gm.partition,
			1,
		)
		clonedSCSIAttachment.mounts[gm.UVMPath] = clonedSCSIMount
	}

	vm.scsiLocations[sm.Controller][sm.LUN] = clonedSCSIAttachment

	return nil
}
