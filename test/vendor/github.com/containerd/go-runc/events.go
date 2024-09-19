/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package runc

// Event is a struct to pass runc event information
type Event struct {
	// Type are the event type generated by runc
	// If the type is "error" then check the Err field on the event for
	// the actual error
	Type  string `json:"type"`
	ID    string `json:"id"`
	Stats *Stats `json:"data,omitempty"`
	// Err has a read error if we were unable to decode the event from runc
	Err error `json:"-"`
}

// Stats is statistical information from the runc process
type Stats struct {
	Cpu     Cpu                `json:"cpu"` //revive:disable
	Memory  Memory             `json:"memory"`
	Pids    Pids               `json:"pids"`
	Blkio   Blkio              `json:"blkio"`
	Hugetlb map[string]Hugetlb `json:"hugetlb"`
}

// Hugetlb represents the detailed hugetlb component of the statistics data
type Hugetlb struct {
	Usage   uint64 `json:"usage,omitempty"`
	Max     uint64 `json:"max,omitempty"`
	Failcnt uint64 `json:"failcnt"`
}

// BlkioEntry represents a block IO entry in the IO stats
type BlkioEntry struct {
	Major uint64 `json:"major,omitempty"`
	Minor uint64 `json:"minor,omitempty"`
	Op    string `json:"op,omitempty"`
	Value uint64 `json:"value,omitempty"`
}

// Blkio represents the statistical information from block IO devices
type Blkio struct {
	IoServiceBytesRecursive []BlkioEntry `json:"ioServiceBytesRecursive,omitempty"`
	IoServicedRecursive     []BlkioEntry `json:"ioServicedRecursive,omitempty"`
	IoQueuedRecursive       []BlkioEntry `json:"ioQueueRecursive,omitempty"`
	IoServiceTimeRecursive  []BlkioEntry `json:"ioServiceTimeRecursive,omitempty"`
	IoWaitTimeRecursive     []BlkioEntry `json:"ioWaitTimeRecursive,omitempty"`
	IoMergedRecursive       []BlkioEntry `json:"ioMergedRecursive,omitempty"`
	IoTimeRecursive         []BlkioEntry `json:"ioTimeRecursive,omitempty"`
	SectorsRecursive        []BlkioEntry `json:"sectorsRecursive,omitempty"`
}

// Pids represents the process ID information
type Pids struct {
	Current uint64 `json:"current,omitempty"`
	Limit   uint64 `json:"limit,omitempty"`
}

// Throttling represents the throttling statistics
type Throttling struct {
	Periods          uint64 `json:"periods,omitempty"`
	ThrottledPeriods uint64 `json:"throttledPeriods,omitempty"`
	ThrottledTime    uint64 `json:"throttledTime,omitempty"`
}

// CpuUsage represents the CPU usage statistics
//
//revive:disable-next-line
type CpuUsage struct {
	// Units: nanoseconds.
	Total  uint64   `json:"total,omitempty"`
	Percpu []uint64 `json:"percpu,omitempty"`
	Kernel uint64   `json:"kernel"`
	User   uint64   `json:"user"`
}

// Cpu represents the CPU usage and throttling statistics
//
//revive:disable-next-line
type Cpu struct {
	Usage      CpuUsage   `json:"usage,omitempty"`
	Throttling Throttling `json:"throttling,omitempty"`
}

// MemoryEntry represents an item in the memory use/statistics
type MemoryEntry struct {
	Limit   uint64 `json:"limit"`
	Usage   uint64 `json:"usage,omitempty"`
	Max     uint64 `json:"max,omitempty"`
	Failcnt uint64 `json:"failcnt"`
}

// Memory represents the collection of memory statistics from the process
type Memory struct {
	Cache     uint64            `json:"cache,omitempty"`
	Usage     MemoryEntry       `json:"usage,omitempty"`
	Swap      MemoryEntry       `json:"swap,omitempty"`
	Kernel    MemoryEntry       `json:"kernel,omitempty"`
	KernelTCP MemoryEntry       `json:"kernelTCP,omitempty"`
	Raw       map[string]uint64 `json:"raw,omitempty"`
}
