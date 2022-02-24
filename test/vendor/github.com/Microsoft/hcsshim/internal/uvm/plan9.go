package uvm

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/osversion"
)

// Plan9Share is a struct containing host paths for the UVM
type Plan9Share struct {
	// UVM resource belongs to
	vm            *UtilityVM
	name, uvmPath string
}

// Release frees the resources of the corresponding Plan9 share
func (p9 *Plan9Share) Release(ctx context.Context) error {
	if err := p9.vm.RemovePlan9(ctx, p9); err != nil {
		return fmt.Errorf("failed to remove plan9 share: %s", err)
	}
	return nil
}

const plan9Port = 564

// AddPlan9 adds a Plan9 share to a utility VM.
func (uvm *UtilityVM) AddPlan9(ctx context.Context, hostPath string, uvmPath string, readOnly bool, restrict bool, allowedNames []string) (*Plan9Share, error) {
	if uvm.operatingSystem != "linux" {
		return nil, errNotSupported
	}
	if restrict && osversion.Build() < osversion.V19H1 {
		return nil, errors.New("single-file mappings are not supported on this build of Windows")
	}
	if uvmPath == "" {
		return nil, fmt.Errorf("uvmPath must be passed to AddPlan9")
	}
	if !readOnly && uvm.NoWritableFileShares() {
		return nil, fmt.Errorf("adding writable shares is denied: %w", hcs.ErrOperationDenied)
	}

	// TODO: JTERRY75 - These are marked private in the schema. For now use them
	// but when there are public variants we need to switch to them.
	const (
		shareFlagsReadOnly           int32 = 0x00000001
		shareFlagsLinuxMetadata      int32 = 0x00000004
		shareFlagsCaseSensitive      int32 = 0x00000008
		shareFlagsRestrictFileAccess int32 = 0x00000080
	)

	// TODO: JTERRY75 - `shareFlagsCaseSensitive` only works if the Windows
	// `hostPath` supports case sensitivity. We need to detect this case before
	// forwarding this flag in all cases.
	flags := shareFlagsLinuxMetadata // | shareFlagsCaseSensitive
	if readOnly {
		flags |= shareFlagsReadOnly
	}
	if restrict {
		flags |= shareFlagsRestrictFileAccess
	}

	uvm.m.Lock()
	index := uvm.plan9Counter
	uvm.plan9Counter++
	uvm.m.Unlock()
	name := strconv.FormatUint(index, 10)

	modification := &hcsschema.ModifySettingRequest{
		RequestType: guestrequest.RequestTypeAdd,
		Settings: hcsschema.Plan9Share{
			Name:         name,
			AccessName:   name,
			Path:         hostPath,
			Port:         plan9Port,
			Flags:        flags,
			AllowedFiles: allowedNames,
		},
		ResourcePath: resourcepaths.Plan9ShareResourcePath,
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedDirectory,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings: guestresource.LCOWMappedDirectory{
				MountPath: uvmPath,
				ShareName: name,
				Port:      plan9Port,
				ReadOnly:  readOnly,
			},
		},
	}

	if err := uvm.modify(ctx, modification); err != nil {
		return nil, err
	}

	return &Plan9Share{
		vm:      uvm,
		name:    name,
		uvmPath: uvmPath,
	}, nil
}

// RemovePlan9 removes a Plan9 share from a utility VM. Each Plan9 share is ref-counted
// and only actually removed when the ref-count drops to zero.
func (uvm *UtilityVM) RemovePlan9(ctx context.Context, share *Plan9Share) error {
	if uvm.operatingSystem != "linux" {
		return errNotSupported
	}

	modification := &hcsschema.ModifySettingRequest{
		RequestType: guestrequest.RequestTypeRemove,
		Settings: hcsschema.Plan9Share{
			Name:       share.name,
			AccessName: share.name,
			Port:       plan9Port,
		},
		ResourcePath: resourcepaths.Plan9ShareResourcePath,
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedDirectory,
			RequestType:  guestrequest.RequestTypeRemove,
			Settings: guestresource.LCOWMappedDirectory{
				MountPath: share.uvmPath,
				ShareName: share.name,
				Port:      plan9Port,
			},
		},
	}
	if err := uvm.modify(ctx, modification); err != nil {
		return fmt.Errorf("failed to remove plan9 share %s from %s: %+v: %s", share.name, uvm.id, modification, err)
	}
	return nil
}
