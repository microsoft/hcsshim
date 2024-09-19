//go:build windows

package processorinfo

import (
	"runtime"

	"github.com/Microsoft/hcsshim/internal/winapi"
)

// ProcessorCount calls the win32 API function GetActiveProcessorCount
// to get the total number of logical processors on the system. If this
// fails it will fall back to runtime.NumCPU
func ProcessorCount() int32 {
	if amount := winapi.GetActiveProcessorCount(winapi.ALL_PROCESSOR_GROUPS); amount != 0 {
		return int32(amount)
	}
	return int32(runtime.NumCPU())
}
