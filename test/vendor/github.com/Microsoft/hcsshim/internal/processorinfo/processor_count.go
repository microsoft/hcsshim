package processorinfo

import "runtime"

// ProcessorCount calls the win32 API function GetMaximumProcessorCount
// to get the total number of logical processors on the system. If this
// fails it will fall back to runtime.NumCPU
func ProcessorCount() int32 {
	if amount := getActiveProcessorCount(ALL_PROCESSOR_GROUPS); amount != 0 {
		return int32(amount)
	}
	return int32(runtime.NumCPU())
}
