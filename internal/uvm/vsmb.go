package uvm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Microsoft/hcsshim/internal/requesttype"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
)

const vsmbSharePrefix = `\\?\VMSMB\VSMB-{dcc079ae-60ba-4d07-847c-3493609c0870}\`

// VSMBShare contains the host path for a Vsmb Mount
type VSMBShare struct {
	// UVM the resource belongs to
	vm           *UtilityVM
	HostPath     string
	refCount     uint32
	name         string
	allowedFiles []string
	guestPath    string
}

// Release frees the resources of the corresponding vsmb Mount
func (vsmb *VSMBShare) Release(ctx context.Context) error {
	if err := vsmb.vm.RemoveVSMB(ctx, vsmb.HostPath); err != nil {
		return fmt.Errorf("failed to remove VSMB share: %s", err)
	}
	return nil
}

// DefaultVSMBOptions returns the default VSMB options. If readOnly is specified,
// returns the default VSMB options for a readonly share.
func (uvm *UtilityVM) DefaultVSMBOptions(readOnly bool) *hcsschema.VirtualSmbShareOptions {
	opts := &hcsschema.VirtualSmbShareOptions{
		NoDirectmap: uvm.DevicesPhysicallyBacked(),
	}
	if readOnly {
		opts.ShareRead = true
		opts.CacheIo = true
		opts.ReadOnly = true
		opts.PseudoOplocks = true
	}
	return opts
}

// findVSMBShare finds a share by `hostPath`. If not found returns `ErrNotAttached`.
func (uvm *UtilityVM) findVSMBShare(ctx context.Context, m map[string]*VSMBShare, hostPath string) (*VSMBShare, error) {
	share, ok := m[hostPath]
	if !ok {
		return nil, ErrNotAttached
	}
	return share, nil
}

// AddVSMB adds a VSMB share to a Windows utility VM. Each VSMB share is ref-counted and
// only added if it isn't already. This is used for read-only layers, mapped directories
// to a container, and for mapped pipes.
func (uvm *UtilityVM) AddVSMB(ctx context.Context, hostPath string, options *hcsschema.VirtualSmbShareOptions) (*VSMBShare, error) {
	if uvm.operatingSystem != "windows" {
		return nil, errNotSupported
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	// Temporary support to allow single-file mapping. If `hostPath` is a
	// directory, map it without restriction. However, if it is a file, map the
	// directory containing the file, and use `AllowedFileList` to only allow
	// access to that file. If the directory has been mapped before for
	// single-file use, add the new file to the `AllowedFileList` and issue an
	// Update operation.
	st, err := os.Stat(hostPath)
	if err != nil {
		return nil, err
	}
	var file string
	m := uvm.vsmbDirShares
	if !st.IsDir() {
		m = uvm.vsmbFileShares
		file = hostPath
		hostPath = filepath.Dir(hostPath)
		options.RestrictFileAccess = true
		options.SingleFileMapping = true
	}
	hostPath = filepath.Clean(hostPath)
	var requestType = requesttype.Update
	share, err := uvm.findVSMBShare(ctx, m, hostPath)
	if err == ErrNotAttached {
		requestType = requesttype.Add
		uvm.vsmbCounter++
		shareName := "s" + strconv.FormatUint(uvm.vsmbCounter, 16)

		share = &VSMBShare{
			vm:        uvm,
			name:      shareName,
			guestPath: vsmbSharePrefix + shareName,
		}
	}
	newAllowedFiles := share.allowedFiles
	if options.RestrictFileAccess {
		newAllowedFiles = append(newAllowedFiles, file)
	}

	// Update on a VSMB share currently only supports updating the
	// AllowedFileList, and in fact will return an error if RestrictFileAccess
	// isn't set (e.g. if used on an unrestricted share). So we only call Modify
	// if we are either doing an Add, or if RestrictFileAccess is set.
	if requestType == requesttype.Add || options.RestrictFileAccess {
		modification := &hcsschema.ModifySettingRequest{
			RequestType: requestType,
			Settings: hcsschema.VirtualSmbShare{
				Name:         share.name,
				Options:      options,
				Path:         hostPath,
				AllowedFiles: newAllowedFiles,
			},
			ResourcePath: vSmbShareResourcePath,
		}
		if err := uvm.modify(ctx, modification); err != nil {
			return nil, err
		}
	}

	share.allowedFiles = newAllowedFiles
	share.refCount++
	m[hostPath] = share
	return share, nil
}

// RemoveVSMB removes a VSMB share from a utility VM. Each VSMB share is ref-counted
// and only actually removed when the ref-count drops to zero.
func (uvm *UtilityVM) RemoveVSMB(ctx context.Context, hostPath string) error {
	if uvm.operatingSystem != "windows" {
		return errNotSupported
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	st, err := os.Stat(hostPath)
	if err != nil {
		return err
	}
	m := uvm.vsmbDirShares
	if !st.IsDir() {
		m = uvm.vsmbFileShares
		hostPath = filepath.Dir(hostPath)
	}
	hostPath = filepath.Clean(hostPath)
	share, err := uvm.findVSMBShare(ctx, m, hostPath)
	if err != nil {
		return fmt.Errorf("%s is not present as a VSMB share in %s, cannot remove", hostPath, uvm.id)
	}

	share.refCount--
	if share.refCount > 0 {
		return nil
	}

	modification := &hcsschema.ModifySettingRequest{
		RequestType:  requesttype.Remove,
		Settings:     hcsschema.VirtualSmbShare{Name: share.name},
		ResourcePath: vSmbShareResourcePath,
	}
	if err := uvm.modify(ctx, modification); err != nil {
		return fmt.Errorf("failed to remove vsmb share %s from %s: %+v: %s", hostPath, uvm.id, modification, err)
	}

	delete(m, hostPath)
	return nil
}

// GetVSMBUvmPath returns the guest path of a VSMB mount.
func (uvm *UtilityVM) GetVSMBUvmPath(ctx context.Context, hostPath string) (string, error) {
	if hostPath == "" {
		return "", fmt.Errorf("no hostPath passed to GetVSMBUvmPath")
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	st, err := os.Stat(hostPath)
	if err != nil {
		return "", err
	}
	m := uvm.vsmbDirShares
	f := ""
	if !st.IsDir() {
		m = uvm.vsmbFileShares
		hostPath, f = filepath.Split(hostPath)
	}
	hostPath = filepath.Clean(hostPath)
	share, err := uvm.findVSMBShare(ctx, m, hostPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(share.guestPath, f), nil
}
