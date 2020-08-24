package winapi

import "syscall"

const PROCESS_ALL_ACCESS uint32 = 0x1FFFFF

const (
	PROC_THREAD_ATTRIBUTE_JOB_LIST       uintptr = 0x2000D
	PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE  uintptr = 0x20016
	PROC_THREAD_ATTRIBUTE_PARENT_PROCESS uintptr = 0x20000
)

// typedef struct _STARTUPINFOEXW {
// 		STARTUPINFOW                 StartupInfo;
// 		LPPROC_THREAD_ATTRIBUTE_LIST lpAttributeList;
// } STARTUPINFOEXW, *LPSTARTUPINFOEXW;
//
type STARTUPINFOEX struct {
	syscall.StartupInfo
	LpAttributeList uintptr
}

// BOOL InitializeProcThreadAttributeList(
// 	LPPROC_THREAD_ATTRIBUTE_LIST lpAttributeList,
// 	DWORD                        dwAttributeCount,
// 	DWORD                        dwFlags,
// 	PSIZE_T                      lpSize
// );
//
//sys InitializeProcThreadAttributeList(lpAttributeList uintptr, dwAttributeCount uint32, dwFlags uint32, lpSize *uintptr) (err error) = kernel32.InitializeProcThreadAttributeList

// BOOL UpdateProcThreadAttribute(
// 	LPPROC_THREAD_ATTRIBUTE_LIST lpAttributeList,
// 	DWORD                        dwFlags,
// 	DWORD_PTR                    Attribute,
// 	PVOID                        lpValue,
// 	SIZE_T                       cbSize,
// 	PVOID                        lpPreviousValue,
// 	PSIZE_T                      lpReturnSize
// );
//
//sys UpdateProcThreadAttribute(lpAttributeList uintptr, dwFlags uint32, attribute uintptr, lpValue *uintptr, cbSize uintptr, lpPreviousValue *uintptr, lpReturnSize *uintptr) (err error) = kernel32.UpdateProcThreadAttribute

// void DeleteProcThreadAttributeList(
// 	LPPROC_THREAD_ATTRIBUTE_LIST lpAttributeList
// );
//
//sys DeleteProcThreadAttributeList(lpAttributeList uintptr) = kernel32.DeleteProcThreadAttributeList
