package winapi

import (
	"syscall"

	"golang.org/x/sys/windows"
)

const UserNameCharLimit = 20

const (
	USER_PRIV_GUEST uint32 = iota
	USER_PRIV_USER
	USER_PRIV_ADMIN
)

const (
	UF_NORMAL_ACCOUNT     = 0x00200
	UF_DONT_EXPIRE_PASSWD = 0x10000
)

const NERR_UserNotFound = syscall.Errno(0x8AD)

// typedef struct _LOCALGROUP_MEMBERS_INFO_0 {
// 	PSID lgrmi0_sid;
// } LOCALGROUP_MEMBERS_INFO_0, *PLOCALGROUP_MEMBERS_INFO_0, *LPLOCALGROUP_MEMBERS_INFO_0;
type LocalGroupMembersInfo0 struct {
	Sid *windows.SID
}

// typedef struct _LOCALGROUP_INFO_1 {
// 	LPWSTR lgrpi1_name;
// 	LPWSTR lgrpi1_comment;
// } LOCALGROUP_INFO_1, *PLOCALGROUP_INFO_1, *LPLOCALGROUP_INFO_1;
type LocalGroupInfo1 struct {
	Name    *uint16
	Comment *uint16
}

// typedef struct _USER_INFO_1 {
// 	LPWSTR usri1_name;
// 	LPWSTR usri1_password;
// 	DWORD  usri1_password_age;
// 	DWORD  usri1_priv;
// 	LPWSTR usri1_home_dir;
// 	LPWSTR usri1_comment;
// 	DWORD  usri1_flags;
// 	LPWSTR usri1_script_path;
// } USER_INFO_1, *PUSER_INFO_1, *LPUSER_INFO_1;
type UserInfo1 struct {
	Name        *uint16
	Password    *uint16
	PasswordAge uint32
	Priv        uint32
	HomeDir     *uint16
	Comment     *uint16
	Flags       uint32
	ScriptPath  *uint16
}

// NET_API_STATUS NET_API_FUNCTION NetLocalGroupGetInfo(
// 	[in]  LPCWSTR servername,
// 	[in]  LPCWSTR groupname,
// 	[in]  DWORD   level,
// 	[out] LPBYTE  *bufptr
// );
//
//sys NetLocalGroupGetInfo(serverName *uint16, groupName *uint16, level uint32, bufptr **byte) (status error) = netapi32.NetLocalGroupGetInfo

// NET_API_STATUS NET_API_FUNCTION NetUserAdd(
// 	[in]  LPCWSTR servername,
// 	[in]  DWORD   level,
// 	[in]  LPBYTE  buf,
// 	[out] LPDWORD parm_err
// );
//
//sys NetUserAdd(serverName *uint16, level uint32, buf *byte, parm_err *uint32) (status error) = netapi32.NetUserAdd

// NET_API_STATUS NET_API_FUNCTION NetUserDel(
// 	[in] LPCWSTR servername,
// 	[in] LPCWSTR username
// );
//
//sys NetUserDel(serverName *uint16, username *uint16) (status error) = netapi32.NetUserDel

// NET_API_STATUS NET_API_FUNCTION NetLocalGroupAddMembers(
// 	[in] LPCWSTR servername,
// 	[in] LPCWSTR groupname,
// 	[in] DWORD   level,
// 	[in] LPBYTE  buf,
// 	[in] DWORD   totalentries
// );
//
//sys NetLocalGroupAddMembers(serverName *uint16, groupName *uint16, level uint32, buf *byte, totalEntries uint32) (status error) = netapi32.NetLocalGroupAddMembers
