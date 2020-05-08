package processorinfo

//go:generate go run ../../mksyscall_windows.go -output zsyscall_windows.go syscall.go

//sys getActiveProcessorCount(groupNumber uint16) (amount uint32) = kernel32.GetActiveProcessorCount

// Get count from all processor groups.
// https://docs.microsoft.com/en-us/windows/win32/procthread/processor-groups
const ALL_PROCESSOR_GROUPS = 0xFFFF
