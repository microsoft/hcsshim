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
	ErrNoAvailableLocation    = fmt.Errorf("no available location")
	ErrNotAttached            = fmt.Errorf("not attached")
	ErrAlreadyAttached        = fmt.Errorf("already attached")
	ErrNoSCSIControllers      = fmt.Errorf("no SCSI controllers configured for this utility VM")
	ErrMoreMountsThanExpected = fmt.Errorf("SCSI attachment has more mounts than expected")
)

func (sm *SCSIMount) Release(ctx context.Context) error {
	if err := sm.vm.RemoveSCSIMount(ctx, sm.HostPath, sm.UVMPath); err != nil {
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

	// A channel to wait on while mount of this SCSI disk is in progress.
	waitCh chan struct{}
	// The error field that is set if the mounting of this disk fails. Any other waiters on waitCh
	// can use this waitErr after the channel is closed.
	waitErr error
}

// SCSIMount
// all UVMPaths must be unique in the uvm
// partition indices can be used in multiple guest mounts
type SCSIMount struct {
	HostPath string
	vm       *UtilityVM

	UVMPath   string
	partition uint8

	refCount uint32

	// A channel to wait on while mount of this SCSI guest mount is in progress.
	waitCh chan struct{}
	// The error field that is set if the mounting of this disk fails. Any other waiters on waitCh
	// can use this waitErr after the channel is closed.
	waitErr error
}

func (sm *SCSIMount) logFormat() logrus.Fields {
	return logrus.Fields{
		"HostPath":  sm.HostPath,
		"UVMPath":   sm.UVMPath,
		"Partition": sm.partition,
		"refCount":  sm.refCount,
	}
}

// addSCSIRequest is an internal struct used to hold all the parameters that are sent to
// the addSCSIActual method.
type addSCSIRequest struct {
	// host path to the disk that should be added as a SCSI disk.
	hostPath string
	// the path inside the uvm at which this disk should show up.
	// Can be empty in attach-only mode.
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
		waitCh:                    make(chan struct{}),
	}
}

func (sa *SCSIAttachment) logFormat() logrus.Fields {
	return logrus.Fields{
		"HostPath":                  sa.HostPath,
		"isLayer":                   sa.isLayer,
		"Mounts":                    sa.mounts,
		"refCount":                  sa.refCount,
		"Controller":                sa.Controller,
		"LUN":                       sa.LUN,
		"ExtensibleVirtualDiskType": sa.extensibleVirtualDiskType,
		"SerialVersionID":           sa.serialVersionID,
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
	if sa, err := uvm.findSCSIAttachment(ctx, hostPath); err == nil {
		sa.refCount++
		return sa, true, nil
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

// GetSCSIMountUVMPath gets the uvm path of a mounted scsi disk
// this should only be used for attachments that do not have multiple non attach-only guest mounts
// UVMPath returned should be for a non attach-only mount, iow should be non empty
func (uvm *UtilityVM) GetSCSIMountUVMPath(ctx context.Context, hostPath string) (string, error) {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	// make sure it's attached
	sa, err := uvm.findSCSIAttachment(ctx, hostPath)
	if err != nil {
		return "", err
	}

	// attachments can have two mounts: one with an empty uvmPath and one with a
	// valid uvmPath
	if len(sa.mounts) > 2 {
		return "", ErrMoreMountsThanExpected
	}

	sm, err := sa.findFirstNonEmptySCSIMount(ctx)
	if err != nil {
		return "", err
	}
	return sm.UVMPath, nil
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

func (sa *SCSIAttachment) GetSerialVersionID() uint32 {
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
		for lun, sa := range uvm.scsiLocations[controller] {
			// If sm is nil, we have found an open slot so we allocate a new SCSIMount
			if sa == nil {
				return controller, lun, nil
			}
		}
	}
	return -1, -1, ErrNoAvailableLocation
}

// deallocateSCSIAttachment is a helper function that removes a SCSIAttachment from a uvm's
// scsi locations.
func (uvm *UtilityVM) deallocateSCSIAttachment(ctx context.Context, sa *SCSIAttachment) {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if sa != nil {
		log.G(ctx).WithFields(sa.logFormat()).Debug("removed SCSI location")
		uvm.scsiLocations[sa.Controller][sa.LUN] = nil
	}
}

// GetSCSIAttachment searches for an attached SCSIAttachment
func (uvm *UtilityVM) GetSCSIAttachment(ctx context.Context, hostPath string) (*SCSIAttachment, error) {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	return uvm.findSCSIAttachment(ctx, hostPath)
}

// findSCSIAttachment is a helper function that searches the uvm's scsi device attachments for an
// attachment with the specified host path.
// Lock must be held when calling this function.
func (uvm *UtilityVM) findSCSIAttachment(ctx context.Context, findThisHostPath string) (*SCSIAttachment, error) {
	for _, luns := range uvm.scsiLocations {
		for _, sa := range luns {
			if sa != nil && sa.HostPath == findThisHostPath {
				log.G(ctx).WithFields(sa.logFormat()).Debug("found SCSI location")
				return sa, nil
			}
		}
	}
	return nil, ErrNotAttached
}

// findSCSIMount is a helper function that searches a SCSIAttachment for a specific
// mount by the host path and uvm path.
// lock must be held when calling this function.
func (sa *SCSIAttachment) findSCSIMount(ctx context.Context, findThisUVMPath string) (*SCSIMount, error) {
	for _, sm := range sa.mounts {
		if sm != nil && sm.UVMPath == findThisUVMPath {
			log.G(ctx).WithFields(sm.logFormat()).Debug("found SCSI mount")
			return sm, nil
		}
	}
	return nil, ErrNotAttached
}

// findFirstNonEmptySCSIMount is similar to findSCSIMount except that it finds the first mount with the
// host path that has a non-empty uvm path.
// lock must be held when calling this function.
func (sa *SCSIAttachment) findFirstNonEmptySCSIMount(ctx context.Context) (*SCSIMount, error) {
	for _, sm := range sa.mounts {
		if sm.UVMPath != "" {
			log.G(ctx).WithFields(sm.logFormat()).Debug("found SCSI mount")
			return sm, nil
		}
	}
	return nil, ErrNotAttached
}

// allocateSCSIMount increments the ref count of an existing scsi mount or allocates a new
// one if not already present.
// Returns the resulting *SCSIMount, a bool indicating if the scsi mount was already present,
// and error if any.
func (uvm *UtilityVM) allocateSCSIMount(
	ctx context.Context,
	sa *SCSIAttachment,
	hostPath string,
	uvmPath string,
	partition uint8,
	allowMultipleGuestMounts bool,
) (_ *SCSIMount, _ bool, err error) {
	uvm.m.Lock()
	defer uvm.m.Unlock()

	var sm *SCSIMount
	if allowMultipleGuestMounts || uvmPath == "" {
		// if we allow multiple scsi mounts for a given hostPath, then we should
		// search by both the hostPath and uvmPath
		// we also allow empty attach-only mounts since they aren't true guest mount
		sm, err = sa.findSCSIMount(ctx, uvmPath)
	} else {
		// else just check if a scsi mount exists only by the hostPath
		sm, err = sa.findFirstNonEmptySCSIMount(ctx)
	}

	if err == nil {
		sm.refCount++
		return sm, true, nil
	}

	scsiMount := newSCSIMount(
		uvm,
		hostPath,
		uvmPath,
		partition,
		1,
	)
	sa.mounts[uvmPath] = scsiMount

	log.G(ctx).WithFields(scsiMount.logFormat()).Debug("allocated SCSI mount")

	return scsiMount, false, nil
}

// deallocateSCSIMount is a helper function that removes a SCSIAttachment's SCSIMount
func (uvm *UtilityVM) deallocateSCSIMount(ctx context.Context, sa *SCSIAttachment, sm *SCSIMount) {
	uvm.m.Lock()
	defer uvm.m.Unlock()
	if sa != nil && sm != nil {
		log.G(ctx).WithFields(sm.logFormat()).Debug("removed SCSI mount")
		sa.mounts[sm.UVMPath] = nil
	}
}

// addSCSIActual is the implementation behind the external functions AddSCSI,
// AddSCSIPhysicalDisk, AddSCSIExtensibleVirtualDisk, AddSCSILayer
//
// We are in control of everything ourselves. Hence we have ref- counting and
// so-on tracking what SCSI locations are available or used.
//
// Returns result from calling modify with the given scsi mount
func (uvm *UtilityVM) addSCSIActual(ctx context.Context, addReq *addSCSIRequest) (_ *SCSIMount, err error) {
	sa, attachmentExisted, err := uvm.allocateSCSIAttachment(
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

	defer func() {
		if err != nil {
			if !attachmentExisted {
				uvm.deallocateSCSIAttachment(ctx, sa)
			} else {
				sa.refCount--
			}
		}

		// if we needed to attach the device, then we need to close the wait channel so other
		// requests can continue
		if !attachmentExisted {
			sa.waitErr = err
			close(sa.waitCh)
		}
	}()

	if attachmentExisted {
		// if the attachment already existed, we need to make sure it's not currently in progress before
		// we continue. Return err if the previous attachment failed.
		<-sa.waitCh
		if sa.waitErr != nil {
			return nil, sa.waitErr
		}
	}

	allowMultipleGuestMounts := addReq.isLayer && uvm.operatingSystem != "windows"
	sm, mountExisted, err := uvm.allocateSCSIMount(
		ctx,
		sa,
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
		<-sm.waitCh
		if sm.waitErr != nil {
			return nil, sm.waitErr
		}
		return sm, nil
	}

	// This is the first goroutine to add this guest mount, close the waitCh after we are done.
	defer func() {
		if err != nil {
			uvm.deallocateSCSIMount(ctx, sa, sm)
		}

		// close the wait channel for the guest mount
		// error must be set _before_ the channel is closed.
		sm.waitErr = err
		close(sm.waitCh)
	}()

	var SCSIModification *hcsschema.ModifySettingRequest = nil

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
			ResourcePath: fmt.Sprintf(resourcepaths.SCSIResourceFormat, guestrequest.ScsiControllerGuids[sa.Controller], sa.LUN),
		}
	}

	if sm.UVMPath != "" {
		if SCSIModification == nil {
			SCSIModification = &hcsschema.ModifySettingRequest{}
		}
		guestReq := guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedVirtualDisk,
			RequestType:  guestrequest.RequestTypeAdd,
		}

		if uvm.operatingSystem == "windows" {
			guestReq.Settings = guestresource.WCOWMappedVirtualDisk{
				ContainerPath: sm.UVMPath,
				Lun:           sa.LUN,
			}
		} else {
			var verity *guestresource.DeviceVerityInfo
			if v, iErr := readVeritySuperBlock(ctx, sm.HostPath); iErr != nil {
				log.G(ctx).WithError(iErr).WithField("hostPath", sm.HostPath).Debug("unable to read dm-verity information from VHD")
			} else {
				if v != nil {
					log.G(ctx).WithFields(logrus.Fields{
						"hostPath":   sm.HostPath,
						"rootDigest": v.RootDigest,
					}).Debug("adding SCSI with dm-verity")
				}
				verity = v
			}

			guestReq.Settings = guestresource.LCOWMappedVirtualDisk{
				MountPath:  sm.UVMPath,
				Lun:        uint8(sa.LUN),
				Controller: uint8(sa.Controller),
				ReadOnly:   addReq.readOnly,
				Encrypted:  addReq.encrypted,
				Options:    addReq.guestOptions,
				VerityInfo: verity,
			}
		}
		SCSIModification.GuestRequest = guestReq
	}

	if SCSIModification == nil {
		// no request to make, only book keeping changes
		return sm, nil
	}

	if err := uvm.modify(ctx, SCSIModification); err != nil {
		return nil, fmt.Errorf("failed to modify UVM with new SCSI mount: %s", err)
	}
	return sm, nil
}

// RemoveSCSI removes a SCSI disk from a utility VM.
func (uvm *UtilityVM) RemoveSCSIMount(ctx context.Context, hostPath, uvmPath string) error {
	uvm.m.Lock()
	defer uvm.m.Unlock()

	if uvm.scsiControllerCount == 0 {
		return ErrNoSCSIControllers
	}

	// Make sure it is actually attached
	sa, err := uvm.findSCSIAttachment(ctx, hostPath)
	if err != nil {
		return err
	}

	// get the mount
	sm, err := sa.findSCSIMount(ctx, uvmPath)
	if err != nil {
		return err
	}

	sa.refCount--
	sm.refCount--
	if sm.refCount > 0 {
		return nil
	}

	removeAttachment := (sa.refCount <= 0)
	var scsiModification *hcsschema.ModifySettingRequest = nil

	if removeAttachment {
		scsiModification = &hcsschema.ModifySettingRequest{
			RequestType:  guestrequest.RequestTypeRemove,
			ResourcePath: fmt.Sprintf(resourcepaths.SCSIResourceFormat, guestrequest.ScsiControllerGuids[sa.Controller], sa.LUN),
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
	if uvm.operatingSystem == "windows" {
		if sm.UVMPath != "" {
			if scsiModification == nil {
				scsiModification = &hcsschema.ModifySettingRequest{}
			}
			scsiModification.GuestRequest = guestrequest.ModificationRequest{
				ResourceType: guestresource.ResourceTypeMappedVirtualDisk,
				RequestType:  guestrequest.RequestTypeRemove,
				Settings: guestresource.WCOWMappedVirtualDisk{
					ContainerPath: sm.UVMPath,
					Lun:           sa.LUN,
				},
			}
		}
	} else {
		if scsiModification == nil {
			scsiModification = &hcsschema.ModifySettingRequest{}
		}
		scsiModification.GuestRequest = guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedVirtualDisk,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings: guestresource.LCOWMappedVirtualDisk{
				MountPath:    sm.UVMPath, // May be blank in attach-only
				Lun:          uint8(sa.LUN),
				Controller:   uint8(sa.Controller),
				VerityInfo:   verity,
				UnplugDevice: removeAttachment,
			},
		}
	}

	if scsiModification == nil {
		// no request needs to be made, just book keeping
		return nil
	}

	if err := uvm.modify(ctx, scsiModification); err != nil {
		return fmt.Errorf("failed to remove SCSI disk %s from container %s: %s", hostPath, uvm.id, err)
	}

	if removeAttachment {
		uvm.scsiLocations[sa.Controller][sa.LUN] = nil
		log.G(ctx).WithFields(sm.logFormat()).Debug("removed SCSI attachment")
	}

	sa.mounts[sm.UVMPath] = nil
	log.G(ctx).WithFields(sm.logFormat()).Debug("removed SCSI mount")

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
func (sa *SCSIAttachment) GobEncode() ([]byte, error) {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	errMsgFmt := "failed to encode SCSIMount: %s"
	// encode only the fields that can be safely deserialized.
	if err := encoder.Encode(sa.serialVersionID); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sa.HostPath); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sa.mounts); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sa.Controller); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sa.LUN); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sa.readOnly); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sa.attachmentType); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sa.extensibleVirtualDiskType); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	if err := encoder.Encode(sa.isLayer); err != nil {
		return nil, fmt.Errorf(errMsgFmt, err)
	}
	return buf.Bytes(), nil
}

// GobDecode deserializes the SCSIMount struct into the struct on which this is called
// (i.e the sm pointer)
func (sa *SCSIAttachment) GobDecode(data []byte) error {
	buf := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buf)
	errMsgFmt := "failed to decode SCSIMount: %s"
	// fields should be decoded in the same order in which they were encoded.
	if err := decoder.Decode(&sa.serialVersionID); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if sa.serialVersionID != scsiCurrentSerialVersionID {
		return fmt.Errorf("serialized version of SCSIMount: %d doesn't match with the current version: %d", sa.serialVersionID, scsiCurrentSerialVersionID)
	}
	if err := decoder.Decode(&sa.HostPath); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if err := decoder.Decode(&sa.mounts); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if err := decoder.Decode(&sa.Controller); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if err := decoder.Decode(&sa.LUN); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if err := decoder.Decode(&sa.readOnly); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if err := decoder.Decode(&sa.attachmentType); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if err := decoder.Decode(&sa.extensibleVirtualDiskType); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	if err := decoder.Decode(&sa.isLayer); err != nil {
		return fmt.Errorf(errMsgFmt, err)
	}
	return nil
}

// Clone function creates a clone of the SCSIAttachment `sa` and adds the cloned SCSIAttachment to
// the uvm `vm`. If `sa` is read only then it is simply added to the `vm`. But if it is a
// writable mount(e.g a scratch layer) then a copy of it is made and that copy is added
// to the `vm`.
func (sa *SCSIAttachment) Clone(ctx context.Context, vm *UtilityVM, cd *cloneData) error {
	var (
		dstVhdPath string = sa.HostPath
		err        error
		dir        string
		conStr     string = guestrequest.ScsiControllerGuids[sa.Controller]
		lunStr     string = fmt.Sprintf("%d", sa.LUN)
	)

	if !sa.readOnly {
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
		if sa.Controller != 0 || sa.LUN != 0 {
			dir, err = os.MkdirTemp(cd.scratchFolder, fmt.Sprintf("clone-mount-%d-%d", sa.Controller, sa.LUN))
			if err != nil {
				return fmt.Errorf("error while creating directory for scsi mounts of clone vm: %s", err)
			}
		}

		// copy the VHDX
		dstVhdPath = filepath.Join(dir, filepath.Base(sa.HostPath))
		log.G(ctx).WithFields(logrus.Fields{
			"source hostPath":      sa.HostPath,
			"controller":           sa.Controller,
			"LUN":                  sa.LUN,
			"destination hostPath": dstVhdPath,
		}).Debug("Creating a clone of SCSI mount")

		if err = copyfile.CopyFile(ctx, sa.HostPath, dstVhdPath, true); err != nil {
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
		Type_: sa.attachmentType,
	}

	clonedSCSIAttachment := newSCSIAttachment(
		dstVhdPath,
		sa.attachmentType,
		sa.extensibleVirtualDiskType,
		1,
		sa.Controller,
		sa.LUN,
		sa.readOnly,
		sa.encrypted,
		sa.isLayer,
	)

	for _, sm := range sa.mounts {
		clonedSCSIMount := newSCSIMount(
			sm.vm,
			dstVhdPath,
			sm.UVMPath,
			sm.partition,
			1,
		)
		clonedSCSIAttachment.mounts[sm.UVMPath] = clonedSCSIMount
	}

	vm.scsiLocations[sa.Controller][sa.LUN] = clonedSCSIAttachment

	return nil
}
