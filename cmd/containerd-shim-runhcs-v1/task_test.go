package main

import (
	"context"
	"time"

	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	v1 "github.com/containerd/cgroups/stats/v1"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/runtime/v2/task"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

var _ = (shimTask)(&testShimTask{})

type testShimTask struct {
	id string

	isWCOW bool
	exec   *testShimExec
	execs  map[string]*testShimExec
}

func (tst *testShimTask) ID() string {
	return tst.id
}

func (tst *testShimTask) CreateExec(ctx context.Context, req *task.ExecProcessRequest, s *specs.Process) error {
	return errdefs.ErrNotImplemented
}

func (tst *testShimTask) GetExec(eid string) (shimExec, error) {
	if eid == "" {
		return tst.exec, nil
	}
	e, ok := tst.execs[eid]
	if ok {
		return e, nil
	}
	return nil, errdefs.ErrNotFound
}

func (tst *testShimTask) KillExec(ctx context.Context, eid string, signal uint32, all bool) error {
	e, err := tst.GetExec(eid)
	if err != nil {
		return err
	}
	return e.Kill(ctx, signal)
}

func (tst *testShimTask) DeleteExec(ctx context.Context, eid string) (int, uint32, time.Time, error) {
	e, err := tst.GetExec(eid)
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	status := e.Status()
	if eid != "" {
		delete(tst.execs, eid)
	}
	return int(status.Pid), status.ExitStatus, status.ExitedAt, nil
}

func (tst *testShimTask) Pids(ctx context.Context) ([]options.ProcessDetails, error) {
	pairs := []options.ProcessDetails{
		{
			ProcessID: uint32(tst.exec.Pid()),
			ExecID:    tst.exec.ID(),
		},
	}
	for _, p := range tst.execs {
		pairs = append(pairs, options.ProcessDetails{
			ProcessID: uint32(p.pid),
			ExecID:    p.id,
		})
	}
	return pairs, nil
}

func (tst *testShimTask) Wait() *task.StateResponse {
	return tst.exec.Wait()
}

func (tst *testShimTask) ExecInHost(ctx context.Context, req *shimdiag.ExecProcessRequest) (int, error) {
	return 0, errors.New("not implemented")
}

func (tst *testShimTask) DumpGuestStacks(ctx context.Context) string {
	return ""
}

func (tst *testShimTask) Share(ctx context.Context, req *shimdiag.ShareRequest) error {
	return errors.New("not implemented")
}

func (tst *testShimTask) Stats(ctx context.Context) (*stats.Statistics, error) {
	if tst.isWCOW {
		return getWCOWTestStats(), nil
	}
	return getLCOWTestStats(), nil
}

func getWCOWTestStats() *stats.Statistics {
	return &stats.Statistics{
		Container: &stats.Statistics_Windows{
			Windows: &stats.WindowsContainerStatistics{
				UptimeNS: 100,
				Processor: &stats.WindowsContainerProcessorStatistics{
					TotalRuntimeNS:  100,
					RuntimeUserNS:   100,
					RuntimeKernelNS: 100,
				},
				Memory: &stats.WindowsContainerMemoryStatistics{
					MemoryUsageCommitBytes:            100,
					MemoryUsageCommitPeakBytes:        100,
					MemoryUsagePrivateWorkingSetBytes: 100,
				},
				Storage: &stats.WindowsContainerStorageStatistics{
					ReadCountNormalized:  100,
					ReadSizeBytes:        100,
					WriteCountNormalized: 100,
					WriteSizeBytes:       100,
				},
			},
		},
		VM: &stats.VirtualMachineStatistics{
			Processor: &stats.VirtualMachineProcessorStatistics{
				TotalRuntimeNS: 100,
			},
			Memory: &stats.VirtualMachineMemoryStatistics{
				WorkingSetBytes: 100,
			},
		},
	}
}

func getLCOWTestStats() *stats.Statistics {
	return &stats.Statistics{
		Container: &stats.Statistics_Linux{
			Linux: &v1.Metrics{
				CPU: &v1.CPUStat{
					Usage: &v1.CPUUsage{
						Total: 100,
					},
				},
				Memory: &v1.MemoryStat{
					TotalInactiveFile: 100,
					Usage: &v1.MemoryEntry{
						Usage: 200,
					},
				},
			},
		},
		VM: &stats.VirtualMachineStatistics{
			Processor: &stats.VirtualMachineProcessorStatistics{
				TotalRuntimeNS: 100,
			},
			Memory: &stats.VirtualMachineMemoryStatistics{
				WorkingSetBytes: 100,
			},
		},
	}
}
