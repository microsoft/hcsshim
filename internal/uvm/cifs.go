package uvm

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/pkg/errors"
)

// CifsMount represents a cifs/smb mount in the Utility VM.
type CifsMount struct {
	vm        *UtilityVM
	mountPath string
	source    string
	refCount  uint
}

// Release removes the cifs mount from the guest.
func (cm *CifsMount) Release(ctx context.Context) error {
	if err := cm.vm.RemoveCifsMount(ctx, cm.source); err != nil {
		return errors.Wrap(err, "failed to remove cifs mount")
	}
	return nil
}

// MountPath returns the location in the UVM the cifs mount has been mounted at.
func (cm *CifsMount) MountPath() string {
	return cm.mountPath
}

// AddCIFSMount adds a cifs mount with address `source` and credentials `username` and
// `password` to the Utility VM.
func (uvm *UtilityVM) AddCIFSMount(ctx context.Context, source, username, password string) (*CifsMount, error) {
	if uvm.operatingSystem != "linux" {
		return nil, errNotSupported
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	existingCM := uvm.cifsMounts[source]
	if existingCM != nil {
		existingCM.refCount++
		return existingCM, nil
	}

	mountPath := fmt.Sprintf(LCOWGlobalMountPrefix, uvm.UVMMountCounter())
	modification := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.GuestRequest{
			ResourceType: guestrequest.ResourceTypeCifsMount,
			RequestType:  requesttype.Add,
			Settings: guestrequest.LCOWCifsMount{
				Source:    source,
				Username:  username,
				Password:  password,
				MountPath: mountPath,
			},
		},
	}

	if err := uvm.modify(ctx, modification); err != nil {
		return nil, err
	}

	cm := &CifsMount{
		vm:        uvm,
		mountPath: mountPath,
		source:    source,
		refCount:  1,
	}

	uvm.cifsMounts[source] = cm
	return cm, nil
}

// RemoveCifsMount removes the cifs mount located at `mountPath` from the vm.
func (uvm *UtilityVM) RemoveCifsMount(ctx context.Context, source string) error {
	if uvm.operatingSystem != "linux" {
		return errNotSupported
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	cm := uvm.cifsMounts[source]
	if cm == nil {
		return fmt.Errorf("no cifs mount with address %s present in the Utility VM", source)
	}

	cm.refCount--
	if cm.refCount == 0 {
		delete(uvm.cifsMounts, source)
		return uvm.modify(ctx, &hcsschema.ModifySettingRequest{
			GuestRequest: guestrequest.GuestRequest{
				ResourceType: guestrequest.ResourceTypeCifsMount,
				RequestType:  requesttype.Remove,
				Settings: guestrequest.LCOWCifsMount{
					MountPath: cm.mountPath,
				},
			},
		})
	}
	return nil
}
