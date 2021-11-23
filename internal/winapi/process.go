package winapi

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

const PROCESS_ALL_ACCESS uint32 = 2097151

const (
	PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE = 0x20016
	PROC_THREAD_ATTRIBUTE_JOB_LIST      = 0x2000D
)

type ProcThreadAttributeList struct {
	_ [1]byte
}

// typedef struct _STARTUPINFOEXW {
// 	STARTUPINFOW                 StartupInfo;
// 	LPPROC_THREAD_ATTRIBUTE_LIST lpAttributeList;
// } STARTUPINFOEXW, *LPSTARTUPINFOEXW;
type StartupInfoEx struct {
	// This is a recreation of the same binding from the stdlib. The x/sys/windows variant for whatever reason
	// doesn't work when updating the list for the pseudo console attribute. It has the process immediately exit
	// with exit code 0xc0000142 shortly after start.
	windows.StartupInfo
	ProcThreadAttributeList *ProcThreadAttributeList
}

// NewProcThreadAttributeList allocates a new ProcThreadAttributeList, with
// the requested maximum number of attributes. This must be cleaned up by calling
// DeleteProcThreadAttributeList.
func NewProcThreadAttributeList(maxAttrCount uint32) (*ProcThreadAttributeList, error) {
	var size uintptr
	err := initializeProcThreadAttributeList(nil, maxAttrCount, 0, &size)
	if err != windows.ERROR_INSUFFICIENT_BUFFER {
		if err == nil {
			return nil, errors.New("unable to query buffer size from InitializeProcThreadAttributeList")
		}
		return nil, err
	}
	al := (*ProcThreadAttributeList)(unsafe.Pointer(&make([]byte, size)[0]))
	err = initializeProcThreadAttributeList(al, maxAttrCount, 0, &size)
	if err != nil {
		return nil, err
	}
	return al, nil
}

// BOOL InitializeProcThreadAttributeList(
// 	[out, optional] LPPROC_THREAD_ATTRIBUTE_LIST lpAttributeList,
// 	[in]            DWORD                        dwAttributeCount,
// 					DWORD                        dwFlags,
// 	[in, out]       PSIZE_T                      lpSize
// );
//
//sys initializeProcThreadAttributeList(lpAttributeList *ProcThreadAttributeList, dwAttributeCount uint32, dwFlags uint32, lpSize *uintptr) (err error) = kernel32.InitializeProcThreadAttributeList

// void DeleteProcThreadAttributeList(
// 	[in, out] LPPROC_THREAD_ATTRIBUTE_LIST lpAttributeList
// );
//
//sys DeleteProcThreadAttributeList(lpAttributeList *ProcThreadAttributeList) = kernel32.DeleteProcThreadAttributeList

// BOOL UpdateProcThreadAttribute(
// 	[in, out]       LPPROC_THREAD_ATTRIBUTE_LIST lpAttributeList,
// 	[in]            DWORD                        dwFlags,
// 	[in]            DWORD_PTR                    Attribute,
// 	[in]            PVOID                        lpValue,
// 	[in]            SIZE_T                       cbSize,
// 	[out, optional] PVOID                        lpPreviousValue,
// 	[in, optional]  PSIZE_T                      lpReturnSize
// );
//
//sys UpdateProcThreadAttribute(lpAttributeList *ProcThreadAttributeList, dwFlags uint32, attribute uintptr, lpValue unsafe.Pointer, cbSize uintptr, lpPreviousValue unsafe.Pointer, lpReturnSize *uintptr) (err error) = kernel32.UpdateProcThreadAttribute

//sys CreateProcessAsUser(token windows.Token, appName *uint16, commandLine *uint16, procSecurity *windows.SecurityAttributes, threadSecurity *windows.SecurityAttributes, inheritHandles bool, creationFlags uint32, env *uint16, currentDir *uint16, startupInfo *windows.StartupInfo, outProcInfo *windows.ProcessInformation) (err error) = advapi32.CreateProcessAsUserW
