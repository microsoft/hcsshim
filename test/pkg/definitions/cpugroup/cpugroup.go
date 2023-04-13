//go:build windows

package cpugroup

import (
	internalcpugroup "github.com/Microsoft/hcsshim/internal/cpugroup"
)

var (
	ErrHVStatusInvalidCPUGroupState = internalcpugroup.ErrHVStatusInvalidCPUGroupState
	Delete                          = internalcpugroup.Delete
	Create                          = internalcpugroup.Create
)
