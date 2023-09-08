//go:build windows

package uvm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/pkg/errors"
)

func (uvm *UtilityVM) AddVsmbAndGetSharePath(ctx context.Context, reqHostPath, reqUVMPath string, readOnly bool) (*VSMBShare, string, error) {
	options := uvm.DefaultVSMBOptions(readOnly)
	vsmbShare, err := uvm.AddVSMB(ctx, reqHostPath, options)
	if err != nil {
		return nil, "", errors.Wrapf(err, "failed to add mount as vSMB share to UVM")
	}
	defer func() {
		if err != nil {
			_ = vsmbShare.Release(ctx)
		}
	}()

	sharePath, err := uvm.GetVSMBUvmPath(ctx, reqHostPath, readOnly)
	if err != nil {
		return nil, "", errors.Wrapf(err, "failed to get vsmb path")
	}

	return vsmbShare, sharePath, nil
}

// Share shares in file(s) from `reqHostPath` on the host machine to `reqUVMPath` inside the UVM.
// This function handles both LCOW and WCOW scenarios.
func (uvm *UtilityVM) Share(ctx context.Context, reqHostPath, reqUVMPath string, readOnly bool) (err error) {
	if uvm.OS() == "windows" {
		_, sharePath, err := uvm.AddVsmbAndGetSharePath(ctx, reqHostPath, reqUVMPath, readOnly)
		if err != nil {
			return err
		}
		guestReq := guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeMappedDirectory,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings: &hcsschema.MappedDirectory{
				HostPath:      sharePath,
				ContainerPath: reqUVMPath,
				ReadOnly:      readOnly,
			},
		}
		if err := uvm.GuestRequest(ctx, guestReq); err != nil {
			return err
		}
	} else {
		st, err := os.Stat(reqHostPath)
		if err != nil {
			return fmt.Errorf("could not open '%s' path on host: %s", reqHostPath, err)
		}
		var (
			hostPath       string = reqHostPath
			restrictAccess bool
			fileName       string
			allowedNames   []string
		)
		if !st.IsDir() {
			hostPath, fileName = filepath.Split(hostPath)
			allowedNames = append(allowedNames, fileName)
			restrictAccess = true
		}
		plan9Share, err := uvm.AddPlan9(ctx, hostPath, reqUVMPath, readOnly, restrictAccess, allowedNames)
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				_ = plan9Share.Release(ctx)
			}
		}()
	}
	return nil
}
