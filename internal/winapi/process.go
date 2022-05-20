package winapi

const PROCESS_ALL_ACCESS uint32 = 2097151

// process attribute values
// see https://docs.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-updateprocthreadattribute
//
//nolint:revive,stylecheck
const (
	PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE                   = 0x20016
	PROC_THREAD_ATTRIBUTE_JOB_LIST                        = 0x2000D
	PROC_THREAD_ATTRIBUTE_CHILD_PROCESS_POLICY            = 0x0002000E
	PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES           = 0x00020009
	PROC_THREAD_ATTRIBUTE_ALL_APPLICATION_PACKAGES_POLICY = 0x0002000F
)

//nolint:revive,stylecheck
const (
	PROCESS_CREATION_MITIGATION_POLICY_DEP_ENABLE           = 0x01
	PROCESS_CREATION_MITIGATION_POLICY_DEP_ATL_THUNK_ENABLE = 0x02
	PROCESS_CREATION_MITIGATION_POLICY_SEHOP_ENABLE         = 0x04

	PROCESS_CREATION_MITIGATION_POLICY2_STRICT_CONTROL_FLOW_GUARD_ALWAYS_ON   = 0x00000001 << 8
	PROCESS_CREATION_MITIGATION_POLICY_HEAP_TERMINATE_ALWAYS_ON               = 0x00000001 << 12
	PROCESS_CREATION_MITIGATION_POLICY_BOTTOM_UP_ASLR_ALWAYS_ON               = 0x00000001 << 16
	PROCESS_CREATION_MITIGATION_POLICY_HIGH_ENTROPY_ASLR_ALWAYS_ON            = 0x00000001 << 20
	PROCESS_CREATION_MITIGATION_POLICY_STRICT_HANDLE_CHECKS_ALWAYS_ON         = 0x00000001 << 24
	PROCESS_CREATION_MITIGATION_POLICY_WIN32K_SYSTEM_CALL_DISABLE_ALWAYS_ON   = 0x00000001 << 28
	PROCESS_CREATION_MITIGATION_POLICY_EXTENSION_POINT_DISABLE_ALWAYS_ON      = 0x00000001 << 32
	PROCESS_CREATION_MITIGATION_POLICY_PROHIBIT_DYNAMIC_CODE_ALWAYS_ON        = 0x00000001 << 36
	PROCESS_CREATION_MITIGATION_POLICY_CONTROL_FLOW_GUARD_ALWAYS_ON           = 0x00000001 << 40
	PROCESS_CREATION_MITIGATION_POLICY_BLOCK_NON_MICROSOFT_BINARIES_ALWAYS_ON = 0x00000001 << 44
	PROCESS_CREATION_MITIGATION_POLICY_FONT_DISABLE_ALWAYS_ON                 = 0x00000001 << 48
	PROCESS_CREATION_MITIGATION_POLICY_IMAGE_LOAD_NO_REMOTE_ALWAYS_ON         = 0x00000001 << 52
	PROCESS_CREATION_MITIGATION_POLICY_IMAGE_LOAD_NO_LOW_LABEL_ALWAYS_ON      = 0x00000001 << 56
	PROCESS_CREATION_MITIGATION_POLICY_IMAGE_LOAD_PREFER_SYSTEM32_ALWAYS_ON   = 0x00000001 << 60

	PROCESS_CREATION_MITIGATION_POLICY2_STRICT_CONTROL_FLOW_GUARD_ALWAYS_OFF   = 0x00000002 << 8
	PROCESS_CREATION_MITIGATION_POLICY_HEAP_TERMINATE_ALWAYS_OFF               = 0x00000002 << 12
	PROCESS_CREATION_MITIGATION_POLICY_BOTTOM_UP_ASLR_ALWAYS_OFF               = 0x00000002 << 16
	PROCESS_CREATION_MITIGATION_POLICY_HIGH_ENTROPY_ASLR_ALWAYS_OFF            = 0x00000002 << 20
	PROCESS_CREATION_MITIGATION_POLICY_STRICT_HANDLE_CHECKS_ALWAYS_OFF         = 0x00000002 << 24
	PROCESS_CREATION_MITIGATION_POLICY_WIN32K_SYSTEM_CALL_DISABLE_ALWAYS_OFF   = 0x00000002 << 28
	PROCESS_CREATION_MITIGATION_POLICY_EXTENSION_POINT_DISABLE_ALWAYS_OFF      = 0x00000002 << 32
	PROCESS_CREATION_MITIGATION_POLICY_PROHIBIT_DYNAMIC_CODE_ALWAYS_OFF        = 0x00000002 << 36
	PROCESS_CREATION_MITIGATION_POLICY_CONTROL_FLOW_GUARD_ALWAYS_OFF           = 0x00000002 << 40
	PROCESS_CREATION_MITIGATION_POLICY_BLOCK_NON_MICROSOFT_BINARIES_ALWAYS_OFF = 0x00000002 << 44
	PROCESS_CREATION_MITIGATION_POLICY_FONT_DISABLE_ALWAYS_OFF                 = 0x00000002 << 48
	PROCESS_CREATION_MITIGATION_POLICY_IMAGE_LOAD_NO_REMOTE_ALWAYS_OFF         = 0x00000002 << 52
	PROCESS_CREATION_MITIGATION_POLICY_IMAGE_LOAD_NO_LOW_LABEL_ALWAYS_OFF      = 0x00000002 << 56
	PROCESS_CREATION_MITIGATION_POLICY_IMAGE_LOAD_PREFER_SYSTEM32_ALWAYS_OFF   = 0x00000002 << 60

	PROCESS_CREATION_MITIGATION_POLICY_FORCE_RELOCATE_IMAGES_ALWAYS_ON_REQ_RELOCS = 0x00000003 << 8
	PROCESS_CREATION_MITIGATION_POLICY_CONTROL_FLOW_GUARD_DEFER                   = 0x00000000 << 40
	PROCESS_CREATION_MITIGATION_POLICY_CONTROL_FLOW_GUARD_EXPORT_SUPPRESSION      = 0x00000003 << 40
)

//nolint:revive,stylecheck
const (
	PROCESS_CREATION_CHILD_PROCESS_RESTRICTED = uint32(0x01)
	PROCESS_CREATION_CHILD_PROCESS_OVERRIDE   = uint32(0x02)
)

// ProcessVmCounters corresponds to the _VM_COUNTERS_EX and _VM_COUNTERS_EX2 structures.
const ProcessVmCounters = 3

// __kernel_entry NTSTATUS NtQueryInformationProcess(
// 	[in]            HANDLE           ProcessHandle,
// 	[in]            PROCESSINFOCLASS ProcessInformationClass,
// 	[out]           PVOID            ProcessInformation,
// 	[in]            ULONG            ProcessInformationLength,
// 	[out, optional] PULONG           ReturnLength
// );
//
//sys NtQueryInformationProcess(processHandle windows.Handle, processInfoClass uint32, processInfo uintptr, processInfoLength uint32, returnLength *uint32) (status uint32) = ntdll.NtQueryInformationProcess

// typedef struct _VM_COUNTERS_EX
// {
//    SIZE_T PeakVirtualSize;
//    SIZE_T VirtualSize;
//    ULONG PageFaultCount;
//    SIZE_T PeakWorkingSetSize;
//    SIZE_T WorkingSetSize;
//    SIZE_T QuotaPeakPagedPoolUsage;
//    SIZE_T QuotaPagedPoolUsage;
//    SIZE_T QuotaPeakNonPagedPoolUsage;
//    SIZE_T QuotaNonPagedPoolUsage;
//    SIZE_T PagefileUsage;
//    SIZE_T PeakPagefileUsage;
//    SIZE_T PrivateUsage;
// } VM_COUNTERS_EX, *PVM_COUNTERS_EX;
//
type VM_COUNTERS_EX struct {
	PeakVirtualSize            uintptr
	VirtualSize                uintptr
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

// typedef struct _VM_COUNTERS_EX2
// {
//    VM_COUNTERS_EX CountersEx;
//    SIZE_T PrivateWorkingSetSize;
//    SIZE_T SharedCommitUsage;
// } VM_COUNTERS_EX2, *PVM_COUNTERS_EX2;
//
type VM_COUNTERS_EX2 struct {
	CountersEx            VM_COUNTERS_EX
	PrivateWorkingSetSize uintptr
	SharedCommitUsage     uintptr
}
