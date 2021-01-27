package uvm

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/guestrequest"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/requesttype"
)

type MountError struct {
	Cim        string
	Op         string
	VolumeGUID guid.GUID
	Err        error
}

func (e *MountError) Error() string {
	s := "cim " + e.Op
	if e.Cim != "" {
		s += " " + e.Cim
	}
	s += " " + e.VolumeGUID.String() + ": " + e.Err.Error()
	return s
}

type cimInfo struct {
	// Unique GUID assigned to a cim.
	cimID guid.GUID
	// ref count for number of times this cim was mounted.
	refCount uint32
}

// MountInUVM mounts the cim at path `uvmCimPath`. Note that the cim file must be already present in the
// uvm at path `uvmCimPath`.
func (uvm *UtilityVM) MountInUVM(ctx context.Context, uvmCimPath string) (_ string, err error) {
	if !strings.HasSuffix(uvmCimPath, ".cim") {
		return "", fmt.Errorf("invalid cim file path: %s", uvmCimPath)
	}
	if !uvm.MountCimSupported() {
		return "", fmt.Errorf("uvm %s doesn't support mounting cim", uvm.ID())
	}
	uvm.cimMountMapLock.Lock()
	defer uvm.cimMountMapLock.Unlock()
	if _, ok := uvm.cimMounts[uvmCimPath]; !ok {
		layerGUID, err := guid.NewV4()
		if err != nil {
			return "", fmt.Errorf("error creating guid: %s", err)
		}
		guestReq := guestrequest.GuestRequest{
			ResourceType: guestrequest.ResourceTypeCimMount,
			RequestType:  requesttype.Add,
			Settings: &hcsschema.CimMount{
				ImagePath:      filepath.Dir(uvmCimPath),
				FileSystemName: filepath.Base(uvmCimPath),
				VolumeGuid:     layerGUID.String(),
				MountFlags:     hcsschema.CimMountFlagEnableDax | hcsschema.CimMountFlagCacheFiles,
			},
		}
		if err := uvm.GuestRequest(ctx, guestReq); err != nil {
			return "", fmt.Errorf("failed to mount the cim: %s", err)
		}

		uvm.cimMounts[uvmCimPath] = &cimInfo{layerGUID, 0}
	}
	ci := uvm.cimMounts[uvmCimPath]
	ci.refCount += 1
	return fmt.Sprintf("\\\\?\\Volume{%s}\\", ci.cimID), nil
}

// Returns the path ("\\?\Volume{GUID}\" format) at which the cim at `uvmCimPath` is
// mounted inside the uvm.  Throws an error if the given cim is not mounted.
func (uvm *UtilityVM) GetCimUvmMountPathNt(uvmCimPath string) (string, error) {
	uvm.cimMountMapLock.Lock()
	defer uvm.cimMountMapLock.Unlock()
	ci, ok := uvm.cimMounts[uvmCimPath]
	if !ok {
		return "", fmt.Errorf("cim %s is not mounted", uvmCimPath)
	}
	return fmt.Sprintf("\\\\?\\Volume{%s}\\", ci.cimID), nil
}

// UnmountFromUVM unmounts the cim at path `uvmCimPath` from inside the uvm.
func (uvm *UtilityVM) UnmountFromUVM(ctx context.Context, uvmCimPath string) error {
	uvm.cimMountMapLock.Lock()
	defer uvm.cimMountMapLock.Unlock()
	ci, ok := uvm.cimMounts[uvmCimPath]
	if !ok {
		return fmt.Errorf("cim not mounted inside the uvm")
	}

	if ci.refCount == 1 {
		guestReq := guestrequest.GuestRequest{
			ResourceType: guestrequest.ResourceTypeCimMount,
			RequestType:  requesttype.Remove,
			Settings: &hcsschema.CimMount{
				ImagePath:      filepath.Dir(uvmCimPath),
				FileSystemName: filepath.Base(uvmCimPath),
				VolumeGuid:     ci.cimID.String(),
			},
		}
		if err := uvm.GuestRequest(ctx, guestReq); err != nil {
			return fmt.Errorf("failed to unmount the cim: %s", err)
		}
		delete(uvm.cimMounts, uvmCimPath)
	} else {
		ci.refCount -= 1
	}
	return nil
}
