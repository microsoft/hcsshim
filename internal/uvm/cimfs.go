//go:build windows
// +build windows

package uvm

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
	"github.com/Microsoft/hcsshim/pkg/cimfs"
	"github.com/sirupsen/logrus"
)

type UVMMountedBlockCIMs struct {
	scsiMounts []*scsi.Mount
	// Volume GUID, inside the UVM this is the volume at which the CIMs are mounted
	volumeGUID guid.GUID
	// SCSI mounts are already ref counted, we only need to ref count the mounted merged CIMs
	refCount uint32
	// UVM in which these mounts are done
	host *UtilityVM
	// reference key used for ref counting
	refKey string
}

func (umb *UVMMountedBlockCIMs) MountedVolumePath() string {
	return fmt.Sprintf(cimfs.VolumePathFormat, umb.volumeGUID)
}

func (umb *UVMMountedBlockCIMs) Release(ctx context.Context) error {
	umb.host.blockCIMMountLock.Lock()
	defer umb.host.blockCIMMountLock.Unlock()

	if umb.refCount > 1 {
		umb.refCount--
		return nil
	}

	guestReq := guestrequest.ModificationRequest{
		ResourceType: guestresource.ResourceTypeWCOWBlockCims,
		RequestType:  guestrequest.RequestTypeRemove,
		Settings: &guestresource.CWCOWBlockCIMMounts{
			VolumeGUID: umb.volumeGUID,
		},
	}
	if err := umb.host.GuestRequest(ctx, guestReq); err != nil {
		return fmt.Errorf("failed to mount the cim: %w", err)
	}

	for i := len(umb.scsiMounts) - 1; i >= 0; i-- {
		if err := umb.scsiMounts[i].Release(ctx); err != nil {
			return err
		}
	}

	// Remove from cache when ref count reaches zero
	delete(umb.host.blockCIMMounts, umb.refKey)
	return nil
}

// mergedCIM can be nil,
// sourceCIMs MUST be in the top to bottom order
func (uvm *UtilityVM) MountBlockCIMs(ctx context.Context, mergedCIM *cimfs.BlockCIM, sourceCIMs []*cimfs.BlockCIM, containerID string) (_ *UVMMountedBlockCIMs, retErr error) {
	if len(sourceCIMs) < 1 {
		return nil, fmt.Errorf("at least 1 source CIM is required")
	}

	uvm.blockCIMMountLock.Lock()
	defer uvm.blockCIMMountLock.Unlock()

	layersToAttach := sourceCIMs
	if mergedCIM != nil {
		layersToAttach = append([]*cimfs.BlockCIM{mergedCIM}, sourceCIMs...)
	}

	// The block path of the mergedCIM (or the block path of the sourceCIM when there
	// is just 1 layer) is sufficient to uniquely identify a block CIM mount done in
	// the UVM. We use that to check if we have already mounted this set of CIMs in
	// the UVM.
	if mountedCIMs, ok := uvm.blockCIMMounts[layersToAttach[0].BlockPath]; ok {
		mountedCIMs.refCount++
		return mountedCIMs, nil
	}

	volumeGUID, err := guid.NewV4()
	if err != nil {
		return nil, fmt.Errorf("generated cim mount GUID: %w", err)
	}

	// TODO(ambarve): When inbox GCS adds support for mounting block CIMs, we should
	// use the appropriate request type for confidential vs regular pods as inbox GCS
	// may not understand the CWCOWBlockCIMMounts type.
	settings := &guestresource.CWCOWBlockCIMMounts{
		BlockCIMs:   []guestresource.BlockCIMDevice{},
		VolumeGUID:  volumeGUID,
		MountFlags:  cimfs.CimMountBlockDeviceCim,
		ContainerID: containerID,
	}

	umb := &UVMMountedBlockCIMs{
		volumeGUID: volumeGUID,
		scsiMounts: []*scsi.Mount{},
		refCount:   1,
		host:       uvm,
		refKey:     layersToAttach[0].BlockPath,
	}

	// Cleanup function to release all SCSI mounts on error
	defer func() {
		if retErr != nil {
			for _, mount := range umb.scsiMounts {
				if cErr := mount.Release(ctx); cErr != nil {
					log.G(ctx).WithFields(logrus.Fields{
						"mount":          mount,
						"original error": retErr,
					}).Debugf("failure during SCSI mount cleanup: %s", cErr)
				}
			}
		}
	}()

	for _, bcim := range layersToAttach {
		sm, err := uvm.SCSIManager.AddVirtualDisk(ctx, bcim.BlockPath, true, uvm.ID(), "", nil)
		if err != nil {
			return nil, fmt.Errorf("failed to attach block CIM %s: %w", bcim.BlockPath, err)
		}

		hasher := sha256.New()
		hasher.Write([]byte(bcim.BlockPath))
		layerDigest := base64.URLEncoding.EncodeToString(hasher.Sum(nil))

		log.G(ctx).WithFields(logrus.Fields{
			"block path":      bcim.BlockPath,
			"cim name":        bcim.CimName,
			"layer digest":    layerDigest,
			"scsi controller": sm.Controller(),
			"scsi LUN":        sm.LUN(),
		}).Debugf("attached block CIM VHD")

		settings.BlockCIMs = append(settings.BlockCIMs, guestresource.BlockCIMDevice{
			CimName: bcim.CimName,
			Lun:     int32(sm.LUN()),
		})
		umb.scsiMounts = append(umb.scsiMounts, sm)
	}

	guestReq := guestrequest.ModificationRequest{
		ResourceType: guestresource.ResourceTypeWCOWBlockCims,
		RequestType:  guestrequest.RequestTypeAdd,
		Settings:     settings,
	}
	if err := uvm.GuestRequest(ctx, guestReq); err != nil {
		return nil, fmt.Errorf("failed to mount the cim: %w", err)
	}

	// Add to cache for future reference counting
	uvm.blockCIMMounts[layersToAttach[0].BlockPath] = umb

	return umb, nil
}
