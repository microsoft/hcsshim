package winapi

// HANDLE GetProcessHeap();
//
//sys GetProcessHeap() (procHeap windows.Handle, err error) = kernel32.GetProcessHeap

// DECLSPEC_ALLOCATOR LPVOID HeapAlloc(
// 	HANDLE hHeap,
// 	DWORD  dwFlags,
// 	SIZE_T dwBytes
// );
//
//sys HeapAlloc(hHeap windows.Handle, dwFlags uint32, dwBytes uintptr) (lpMem uintptr, err error) = kernel32.HeapAlloc

// BOOL HeapFree(
// 	HANDLE                 hHeap,
// 	DWORD                  dwFlags,
// 	_Frees_ptr_opt_ LPVOID lpMem
// );
//
//sys HeapFree(hHeap windows.Handle, dwFlags uint32, lpMem uintptr) (err error) = kernel32.HeapFree
