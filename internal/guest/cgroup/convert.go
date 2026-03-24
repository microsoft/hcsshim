//go:build linux
// +build linux

package cgroup

import (
	cgroups1stats "github.com/containerd/cgroups/v3/cgroup1/stats"
	cgroups2 "github.com/containerd/cgroups/v3/cgroup2"
	cgroups2stats "github.com/containerd/cgroups/v3/cgroup2/stats"
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

// ConvertV2StatsToV1 converts cgroup v2 metrics to v1 metrics format for compatibility.
func ConvertV2StatsToV1(v2Stats *cgroups2stats.Metrics) *cgroups1stats.Metrics {
	if v2Stats == nil {
		return &cgroups1stats.Metrics{}
	}

	v1Stats := &cgroups1stats.Metrics{}

	if v2Stats.Memory != nil {
		v1Stats.Memory = &cgroups1stats.MemoryStat{
			Usage: &cgroups1stats.MemoryEntry{
				Usage: v2Stats.Memory.Usage,
				Limit: v2Stats.Memory.UsageLimit,
				Max:   v2Stats.Memory.MaxUsage,
			},
			Swap: &cgroups1stats.MemoryEntry{
				Usage: v2Stats.Memory.SwapUsage,
				Limit: v2Stats.Memory.SwapLimit,
				Max:   v2Stats.Memory.SwapMaxUsage,
			},
			Kernel: &cgroups1stats.MemoryEntry{
				Usage: v2Stats.Memory.KernelStack,
				Limit: 0,
				Max:   0,
			},
			KernelTCP: &cgroups1stats.MemoryEntry{
				Usage: v2Stats.Memory.Sock,
				Limit: 0,
				Max:   0,
			},
			HierarchicalMemoryLimit: v2Stats.Memory.UsageLimit,
			HierarchicalSwapLimit:   v2Stats.Memory.SwapLimit,
			RSS:                     v2Stats.Memory.Anon,
			Cache:                   v2Stats.Memory.File,
			MappedFile:              v2Stats.Memory.FileMapped,
			Dirty:                   v2Stats.Memory.FileDirty,
			Writeback:               v2Stats.Memory.FileWriteback,
			PgFault:                 v2Stats.Memory.Pgfault,
			PgMajFault:              v2Stats.Memory.Pgmajfault,
			InactiveAnon:            v2Stats.Memory.InactiveAnon,
			ActiveAnon:              v2Stats.Memory.ActiveAnon,
			InactiveFile:            v2Stats.Memory.InactiveFile,
			ActiveFile:              v2Stats.Memory.ActiveFile,
			Unevictable:             v2Stats.Memory.Unevictable,
		}
	}

	if v2Stats.MemoryEvents != nil {
		v1Stats.MemoryOomControl = &cgroups1stats.MemoryOomControl{
			OomKill:        v2Stats.MemoryEvents.OomKill,
			OomKillDisable: 0,
			UnderOom:       0,
		}
	}

	if v2Stats.CPU != nil {
		v1Stats.CPU = &cgroups1stats.CPUStat{
			Usage: &cgroups1stats.CPUUsage{
				Total:  v2Stats.CPU.UsageUsec * 1000,
				User:   v2Stats.CPU.UserUsec * 1000,
				Kernel: v2Stats.CPU.SystemUsec * 1000,
			},
			Throttling: &cgroups1stats.Throttle{
				Periods:          v2Stats.CPU.NrPeriods,
				ThrottledPeriods: v2Stats.CPU.NrThrottled,
				ThrottledTime:    v2Stats.CPU.ThrottledUsec * 1000,
			},
		}
	}

	if v2Stats.Io != nil && len(v2Stats.Io.Usage) > 0 {
		v1Stats.Blkio = &cgroups1stats.BlkIOStat{
			IoServiceBytesRecursive: make([]*cgroups1stats.BlkIOEntry, 0, len(v2Stats.Io.Usage)*2),
			IoServicedRecursive:     make([]*cgroups1stats.BlkIOEntry, 0, len(v2Stats.Io.Usage)*2),
		}

		for _, entry := range v2Stats.Io.Usage {
			v1Stats.Blkio.IoServiceBytesRecursive = append(
				v1Stats.Blkio.IoServiceBytesRecursive,
				&cgroups1stats.BlkIOEntry{Major: entry.Major, Minor: entry.Minor, Op: "Read", Value: entry.Rbytes},
				&cgroups1stats.BlkIOEntry{Major: entry.Major, Minor: entry.Minor, Op: "Write", Value: entry.Wbytes},
			)
			v1Stats.Blkio.IoServicedRecursive = append(
				v1Stats.Blkio.IoServicedRecursive,
				&cgroups1stats.BlkIOEntry{Major: entry.Major, Minor: entry.Minor, Op: "Read", Value: entry.Rios},
				&cgroups1stats.BlkIOEntry{Major: entry.Major, Minor: entry.Minor, Op: "Write", Value: entry.Wios},
			)
		}
	}

	if v2Stats.Pids != nil {
		v1Stats.Pids = &cgroups1stats.PidsStat{
			Current: v2Stats.Pids.Current,
			Limit:   v2Stats.Pids.Limit,
		}
	}

	if len(v2Stats.Hugetlb) > 0 {
		v1Stats.Hugetlb = make([]*cgroups1stats.HugetlbStat, len(v2Stats.Hugetlb))
		for i, stats := range v2Stats.Hugetlb {
			v1Stats.Hugetlb[i] = &cgroups1stats.HugetlbStat{
				Usage:   stats.Current,
				Max:     stats.Max,
				Failcnt: 0,
			}
		}
	}

	if v2Stats.Rdma != nil {
		v1Stats.Rdma = &cgroups1stats.RdmaStat{}
		if len(v2Stats.Rdma.Current) > 0 {
			v1Stats.Rdma.Current = make([]*cgroups1stats.RdmaEntry, len(v2Stats.Rdma.Current))
			for i, entry := range v2Stats.Rdma.Current {
				v1Stats.Rdma.Current[i] = &cgroups1stats.RdmaEntry{
					Device:     entry.Device,
					HcaHandles: entry.HcaHandles,
					HcaObjects: entry.HcaObjects,
				}
			}
		}
		if len(v2Stats.Rdma.Limit) > 0 {
			v1Stats.Rdma.Limit = make([]*cgroups1stats.RdmaEntry, len(v2Stats.Rdma.Limit))
			for i, entry := range v2Stats.Rdma.Limit {
				v1Stats.Rdma.Limit[i] = &cgroups1stats.RdmaEntry{
					Device:     entry.Device,
					HcaHandles: entry.HcaHandles,
					HcaObjects: entry.HcaObjects,
				}
			}
		}
	}

	// Note: cgroup v2 does not expose network stats (no Network field in cgroup2/stats.Metrics).
	// Network stats are typically collected from /proc/net/dev or netlink instead.

	return v1Stats
}

// ConvertToV2Resources converts oci.LinuxResources to cgroups2.Resources.
func ConvertToV2Resources(resources *oci.LinuxResources) *cgroups2.Resources {
	if resources == nil {
		return &cgroups2.Resources{}
	}
	v2Resources := &cgroups2.Resources{}

	if resources.Memory != nil {
		v2Memory := &cgroups2.Memory{}
		if resources.Memory.Limit != nil {
			v2Memory.Max = resources.Memory.Limit
		}
		if resources.Memory.Reservation != nil {
			v2Memory.Low = resources.Memory.Reservation
		}
		if resources.Memory.Swap != nil {
			v2Memory.Swap = resources.Memory.Swap
		}
		v2Resources.Memory = v2Memory
	}

	if resources.CPU != nil {
		v2CPU := &cgroups2.CPU{}
		if resources.CPU.Shares != nil && *resources.CPU.Shares > 0 {
			// Convert v1 cpu.shares (range 2-262144, default 1024) to v2 cpu.weight
			// (range 1-10000, default 100) using the community-standard formula from runc.
			// Shares=0 means "use default"; we leave Weight unset so v2 uses its kernel default.
			shares := *resources.CPU.Shares
			if shares < 2 {
				shares = 2
			}
			weight := uint64(1 + ((shares-2)*9999)/262142)
			v2CPU.Weight = &weight
		}
		if resources.CPU.Quota != nil && resources.CPU.Period != nil {
			v2CPU.Max = cgroups2.NewCPUMax(resources.CPU.Quota, resources.CPU.Period)
		}
		v2Resources.CPU = v2CPU
	}

	if resources.Pids != nil && resources.Pids.Limit > 0 {
		v2Resources.Pids = &cgroups2.Pids{
			Max: resources.Pids.Limit,
		}
	}

	if resources.BlockIO != nil {
		v2IO := &cgroups2.IO{}
		if resources.BlockIO.Weight != nil {
			v2IO.BFQ.Weight = *resources.BlockIO.Weight
		}
		v2Resources.IO = v2IO
	}

	return v2Resources
}
