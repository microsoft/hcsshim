//go:build windows

package uvm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"unsafe"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/Microsoft/hcsshim/osversion"
)

const (
	vsmbSharePrefix = `\\?\VMSMB\VSMB-{dcc079ae-60ba-4d07-847c-3493609c0870}\`
)

// VSMBShare contains the host path for a Vsmb Mount.
type VSMBShare struct {
	// UVM the resource belongs to.
	vm           *UtilityVM
	HostPath     string
	refCount     uint32
	name         string
	allowedFiles []string
	guestPath    string
	options      hcsschema.VirtualSmbShareOptions
	// whether the share is mapping an entire directory.
	// ie, if the share is stored in [vm.vsmbDirShares] or [vm.vsmbFileShares].
	isDirShare bool
}

// Release frees the resources of the corresponding vsmb Mount
func (vsmb *VSMBShare) Release(ctx context.Context) error {
	if err := vsmb.vm.removeVSMB(ctx, vsmb.HostPath, vsmb.options.ReadOnly, vsmb.isDirShare); err != nil {
		return fmt.Errorf("failed to remove VSMB share: %w", err)
	}
	return nil
}

// DefaultVSMBOptions returns the default VSMB options. If readOnly is specified,
// returns the default VSMB options for a readonly share.
func (uvm *UtilityVM) DefaultVSMBOptions(readOnly bool) *hcsschema.VirtualSmbShareOptions {
	opts := &hcsschema.VirtualSmbShareOptions{
		NoDirectmap: uvm.DevicesPhysicallyBacked() || uvm.VSMBNoDirectMap(),
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
func (*UtilityVM) findVSMBShare(_ context.Context, m map[string]*VSMBShare, shareKey string) (*VSMBShare, error) {
	share, ok := m[shareKey]
	if !ok {
		return nil, ErrNotAttached
	}
	return share, nil
}

// openHostPath opens the given path and returns the handle. The handle is opened with
// full sharing and no access mask. The directory must already exist. This
// function is intended to return a handle suitable for use with GetFileInformationByHandleEx.
//
// We are not able to use builtin Go functionality for opening a directory path:
//
//   - os.Open on a directory returns a os.File where Fd() is a search handle from FindFirstFile.
//   - syscall.Open does not provide a way to specify FILE_FLAG_BACKUP_SEMANTICS, which is needed to
//     open a directory.
//
// We could use os.Open if the path is a file, but it's easier to just use the same code for both.
// Therefore, we call windows.CreateFile directly.
func openHostPath(path string) (windows.Handle, error) {
	u16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateFile(
		u16,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0)
	if err != nil {
		return 0, &os.PathError{
			Op:   "CreateFile",
			Path: path,
			Err:  err,
		}
	}
	return h, nil
}

// In 19H1, a change was made to VSMB to require querying file ID for the files being shared in
// order to support direct map. This change was made to ensure correctness in cases where direct
// map is used with saving/restoring VMs.
//
// However, certain file systems (such as Azure Files SMB shares) don't support the FileIdInfo
// query that is used. Azure Files in particular fails with ERROR_INVALID_PARAMETER. This issue
// affects at least 19H1, 19H2, 20H1, and 20H2.
//
// To work around this, we attempt to query for FileIdInfo ourselves if on an affected build. If
// the query fails, we override the specified options to force no direct map to be used.
func forceNoDirectMap(path string) (bool, error) {
	if ver := osversion.Build(); ver < osversion.V19H1 || ver > osversion.V20H2 {
		return false, nil
	}
	h, err := openHostPath(path)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = windows.CloseHandle(h)
	}()
	var info winapi.FILE_ID_INFO
	// We check for any error, rather than just ERROR_INVALID_PARAMETER. It seems better to also
	// fall back if e.g. some other backing filesystem is used which returns a different error.
	if err := windows.GetFileInformationByHandleEx(h, winapi.FileIdInfo, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return true, nil
	}
	return false, nil
}

// AddVSMB adds a VSMB share to a Windows utility VM. Each VSMB share is ref-counted and
// only added if it isn't already. This is used for read-only layers, mapped directories
// to a container, and for mapped pipes.
func (uvm *UtilityVM) AddVSMB(ctx context.Context, hostPath string, options *hcsschema.VirtualSmbShareOptions) (*VSMBShare, error) {
	if uvm.operatingSystem != "windows" {
		return nil, errNotSupported
	}

	if !options.ReadOnly && uvm.NoWritableFileShares() {
		return nil, fmt.Errorf("adding writable shares is denied: %w", hcs.ErrOperationDenied)
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

	if force, err := forceNoDirectMap(hostPath); err != nil {
		return nil, err
	} else if force {
		log.G(ctx).WithField("path", hostPath).Info("Forcing NoDirectmap for VSMB mount")
		options.NoDirectmap = true
	}

	var requestType = guestrequest.RequestTypeUpdate
	shareKey := getVSMBShareKey(hostPath, options.ReadOnly)
	share, err := uvm.findVSMBShare(ctx, m, shareKey)
	if errors.Is(err, ErrNotAttached) {
		requestType = guestrequest.RequestTypeAdd
		uvm.vsmbCounter++
		shareName := "s" + strconv.FormatUint(uvm.vsmbCounter, 16)

		share = &VSMBShare{
			vm:         uvm,
			name:       shareName,
			guestPath:  vsmbSharePrefix + shareName,
			HostPath:   hostPath,
			isDirShare: st.IsDir(),
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
	if requestType == guestrequest.RequestTypeAdd || options.RestrictFileAccess {
		log.G(ctx).WithFields(logrus.Fields{
			"name":      share.name,
			"path":      hostPath,
			"options":   fmt.Sprintf("%+#v", options),
			"operation": requestType,
		}).Info("Modifying VSMB share")
		modification := &hcsschema.ModifySettingRequest{
			RequestType: requestType,
			Settings: hcsschema.VirtualSmbShare{
				Name:         share.name,
				Options:      options,
				Path:         hostPath,
				AllowedFiles: newAllowedFiles,
			},
			ResourcePath: resourcepaths.VSMBShareResourcePath,
		}
		if err := uvm.modify(ctx, modification); err != nil {
			return nil, err
		}
	}

	share.allowedFiles = newAllowedFiles
	share.refCount++
	share.options = *options
	m[shareKey] = share
	return share, nil
}

// RemoveVSMB removes a VSMB share from a utility VM. Each VSMB share is ref-counted
// and only actually removed when the ref-count drops to zero.
func (uvm *UtilityVM) RemoveVSMB(ctx context.Context, hostPath string, readOnly bool) error {
	st, err := os.Stat(hostPath)
	if err != nil {
		return err
	}
	isDir := st.IsDir()
	if !isDir {
		hostPath = filepath.Dir(hostPath)
	}
	return uvm.removeVSMB(ctx, hostPath, readOnly, isDir)
}

// removeVSMB removes the share for the directory at hostPath.
//
// directoryShare indicates if the share is stored in [uvm.vsmbDirShares] or [uvm.vsmbFileShares].
// Ie, it should match [VSMBShare.isDirShare].
//
// Regardless of whether the vSMB share is mapping a file or directory, hostPath must be the
// directory that was shared into the uVM (and the keyname the [VSMBShare] in either
// [uvm.vsmbDirShares] or [uvm.vsmbFileShares]).
func (uvm *UtilityVM) removeVSMB(ctx context.Context, hostPath string, readOnly, directoryShare bool) error {
	if uvm.operatingSystem != "windows" {
		return errNotSupported
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	m := uvm.vsmbDirShares
	if !directoryShare {
		m = uvm.vsmbFileShares
	}
	hostPath = filepath.Clean(hostPath)
	shareKey := getVSMBShareKey(hostPath, readOnly)
	share, err := uvm.findVSMBShare(ctx, m, shareKey)
	if err != nil {
		return fmt.Errorf("%s is not present as a VSMB share in %s, cannot remove", hostPath, uvm.id)
	}

	share.refCount--
	if share.refCount > 0 {
		return nil
	}

	modification := &hcsschema.ModifySettingRequest{
		RequestType:  guestrequest.RequestTypeRemove,
		Settings:     hcsschema.VirtualSmbShare{Name: share.name},
		ResourcePath: resourcepaths.VSMBShareResourcePath,
	}
	if err := uvm.modify(ctx, modification); err != nil {
		return fmt.Errorf("failed to remove vsmb share %s from %s: %+v: %w", hostPath, uvm.id, modification, err)
	}

	delete(m, shareKey)
	return nil
}

// GetVSMBUvmPath returns the guest path of a VSMB mount.
func (uvm *UtilityVM) GetVSMBUvmPath(ctx context.Context, hostPath string, readOnly bool) (string, error) {
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
	shareKey := getVSMBShareKey(hostPath, readOnly)
	share, err := uvm.findVSMBShare(ctx, m, shareKey)
	if err != nil {
		return "", err
	}
	return filepath.Join(share.guestPath, f), nil
}

// getVSMBShareKey returns a string key which encapsulates the information that is used to
// look up an existing VSMB share. If a share is being added, but there is an existing
// share with the same key, the existing share will be used instead (and its ref count
// incremented).
func getVSMBShareKey(hostPath string, readOnly bool) string {
	return fmt.Sprintf("%v-%v", hostPath, readOnly)
}
