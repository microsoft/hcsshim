package winapi

const PROCESS_ALL_ACCESS uint32 = 2097151

// DWORD GetProcessImageFileNameW(
//	HANDLE hProcess,
//	LPWSTR lpImageFileName,
//	DWORD  nSize
// );
//sys GetProcessImageFileName(hProcess windows.Handle, imageFileName *uint16, nSize uint32) (size uint32, err error) = kernel32.GetProcessImageFileNameW

//sys CreateProcessAsUser(token windows.Token, appName *uint16, commandLine *uint16, procSecurity *windows.SecurityAttributes, threadSecurity *windows.SecurityAttributes, inheritHandles bool, creationFlags uint32, env *uint16, currentDir *uint16, startupInfo *windows.StartupInfo, outProcInfo *windows.ProcessInformation) (err error) = advapi32.CreateProcessAsUserW
