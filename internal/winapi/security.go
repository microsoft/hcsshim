package winapi

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func GrantSIDFileAccess(name string, sid *windows.SID, access windows.ACCESS_MASK) error {
	isDir, err := IsDir(name)
	if err != nil {
		return fmt.Errorf("check if %q is directory: %w", name, err)
	}

	inh := uint32(windows.NO_INHERITANCE)
	if isDir {
		inh = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	h, err := openFile(name, isDir, true)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h) //nolint:errcheck
	return GrantSIDHandleAccess(h, sid, access, inh, windows.SE_FILE_OBJECT)
}

func GrantSIDHandleAccess(
	h windows.Handle,
	sid *windows.SID,
	access windows.ACCESS_MASK,
	inheritance uint32,
	t windows.SE_OBJECT_TYPE,
) error {
	eas := []windows.EXPLICIT_ACCESS{
		AllowAccessForSID(sid, access, inheritance),
	}
	return UpdateHandleDACL(h, eas, t)
}

func RevokeSIDFileAccess(name string, sid *windows.SID) error {
	isDir, err := IsDir(name)
	if err != nil {
		return fmt.Errorf("check if %q is directory: %w", name, err)
	}

	inh := uint32(windows.NO_INHERITANCE)
	if isDir {
		inh = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	h, err := openFile(name, isDir, true)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h) //nolint:errcheck
	return RevokeSIDHandleAccess(h, sid, inh, windows.SE_FILE_OBJECT)
}

func RevokeSIDHandleAccess(
	h windows.Handle,
	sid *windows.SID,
	inheritance uint32,
	t windows.SE_OBJECT_TYPE,
) error {
	eas := []windows.EXPLICIT_ACCESS{
		RevokeAccessForSID(sid, inheritance),
	}
	return UpdateHandleDACL(h, eas, t)
}

func UpdateFileDACL(name string, eas []windows.EXPLICIT_ACCESS) error {
	isDir, err := IsDir(name)
	if err != nil {
		return fmt.Errorf("check if %q is directory: %w", name, err)
	}

	h, err := openFile(name, isDir, true)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h) //nolint:errcheck
	return UpdateHandleDACL(h, eas, windows.SE_FILE_OBJECT)
}

func UpdateHandleDACL(h windows.Handle, eas []windows.EXPLICIT_ACCESS, t windows.SE_OBJECT_TYPE) error {
	if len(eas) == 0 {
		return nil
	}

	acl, err := GetHandleDACL(h, t)
	if err != nil {
		return err
	}

	acl, err = windows.ACLFromEntries(eas, acl)
	if err != nil {
		return fmt.Errorf("merging DACL with explicit access entries : %w", err)
	}

	return windows.SetSecurityInfo(h, t, windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION), nil, nil, acl, nil)
}

// GetFileDACL returns the discretional access control list for the file or directory.
func GetFileDACL(name string) (*windows.ACL, error) {
	sd, err := GetFileSD(name)
	if err != nil {
		return nil, err
	}
	acl, _, err := sd.DACL()
	return acl, err
}

func GetFileSD(name string) (*windows.SECURITY_DESCRIPTOR, error) {
	isDir, err := IsDir(name)
	if err != nil {
		return nil, fmt.Errorf("check if %q is directory: %w", name, err)
	}

	h, err := openFile(name, false, isDir)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(h) //nolint:errcheck
	return GetHandleSD(h, windows.SE_FILE_OBJECT)
}

func openFile(name string, isDir, write bool) (windows.Handle, error) {
	da := uint32(windows.READ_CONTROL)
	if write {
		da |= windows.WRITE_DAC
	}
	fa := uint32(windows.FILE_ATTRIBUTE_NORMAL)
	if isDir {
		fa |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}

	h, err := CreateFile(
		name,
		da,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, // security attributes
		windows.OPEN_EXISTING,
		fa,
		0, //template file
	)
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("CreateFile %s: %w", name, err)
	}
	return h, nil
}

func GetHandleDACL(h windows.Handle, t windows.SE_OBJECT_TYPE) (*windows.ACL, error) {
	sd, err := GetHandleSD(h, t)
	if err != nil {
		return nil, err
	}
	acl, _, err := sd.DACL()
	return acl, err
}

func GetHandleSD(h windows.Handle, t windows.SE_OBJECT_TYPE) (*windows.SECURITY_DESCRIPTOR, error) {
	sd, err := windows.GetSecurityInfo(h, t, windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION))
	if err != nil {
		return nil, fmt.Errorf("get security info: %w", err)
	}
	return sd, nil
}

func NewSecurityAttributes(descriptor *windows.SECURITY_DESCRIPTOR, inherit bool) *windows.SecurityAttributes {
	i := uint32(0)
	if inherit {
		i = 1
	}
	sa := &windows.SecurityAttributes{
		SecurityDescriptor: descriptor,
		InheritHandle:      i,
	}
	sa.Length = uint32(unsafe.Sizeof(sa))
	return sa
}

func AllowAccessForSID(sid *windows.SID, access windows.ACCESS_MASK, inheritance uint32) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: access,
		AccessMode:        windows.SET_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_UNKNOWN,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}

func RevokeAccessForSID(sid *windows.SID, inheritance uint32) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessMode:  windows.REVOKE_ACCESS,
		Inheritance: inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_UNKNOWN,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}
