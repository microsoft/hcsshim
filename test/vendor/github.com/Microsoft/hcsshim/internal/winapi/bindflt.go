package winapi

const (
	BINDFLT_FLAG_READ_ONLY_MAPPING        uint32 = 0x00000001
	BINDFLT_FLAG_MERGED_BIND_MAPPING      uint32 = 0x00000002
	BINDFLT_FLAG_USE_CURRENT_SILO_MAPPING uint32 = 0x00000004
)

// HRESULT
// BfSetupFilterEx(
//     _In_ ULONG Flags,
//     _In_opt_ HANDLE JobHandle,
//     _In_opt_ PSID Sid,
//     _In_ LPCWSTR VirtualizationRootPath,
//     _In_ LPCWSTR VirtualizationTargetPath,
//     _In_reads_opt_( VirtualizationExceptionPathCount ) LPCWSTR* VirtualizationExceptionPaths,
//     _In_opt_ ULONG VirtualizationExceptionPathCount
// );
//
//sys BfSetupFilterEx(flags uint32, jobHandle windows.Handle, sid *windows.SID, virtRootPath *uint16, virtTargetPath *uint16, virtExceptions **uint16, virtExceptionPathCount uint32) (hr error) = bindfltapi.BfSetupFilterEx?
