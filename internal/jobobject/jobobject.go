package jobobject

import (
	"context"
	"fmt"
	"sync"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/queue"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// This file provides higher level constructs for the win32 job object API.
// Most of the core creation and management functions are already present in "golang.org/x/sys/windows"
// (CreateJobObject, AssignProcessToJobObject, etc.) as well as most of the limit information
// structs and associated limit flags. Whatever is not present from the job object API
// in golang.org/x/sys/windows is located in /internal/winapi.
//
// https://docs.microsoft.com/en-us/windows/win32/procthread/job-objects

// JobObject is a high level wrapper around a Windows job object. Holds a handle to
// the job, a queue to receive iocp notifications about the lifecycle
// of the job and a mutex for synchronized handle access.
type JobObject struct {
	handle     windows.Handle
	mq         *queue.MessageQueue
	handleLock sync.RWMutex
}

// JobLimits represents the resource constraints that can be applied to a job object.
type JobLimits struct {
	CPULimit           uint32
	CPUWeight          uint32
	MemoryLimitInBytes uint64
	MaxIOPS            int64
	MaxBandwidth       int64
}

type CPURateControlType uint32

const (
	WeightBased CPURateControlType = iota
	RateBased
)

// Processor resource controls
const (
	CPULimitMin  = 1
	CPULimitMax  = 10000
	CPUWeightMin = 1
	CPUWeightMax = 9
)

var (
	ErrAlreadyClosed = errors.New("the handle has already been closed")
	ErrNotRegistered = errors.New("job is not registered to receive notifications")
)

// Create creates a job object.
//
// `name` specifies the name of the job object if a named job object is desired. If name
// is an empty string, the job will not be assigned a name.
//
// `notifications` specifies if the job will be registered to receive notifications.
// If this is false, `PollNotifications` will return immediately with error `errNotRegistered`.
//
// Returns a JobObject structure and an error if there is one.
func Create(ctx context.Context, name string, notifications bool) (_ *JobObject, err error) {
	var (
		jobName *uint16
		mq      *queue.MessageQueue
	)

	if name != "" {
		jobName, err = windows.UTF16PtrFromString(name)
		if err != nil {
			return nil, err
		}
	}

	jobHandle, err := windows.CreateJobObject(nil, jobName)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			windows.Close(jobHandle)
		}
	}()

	// If the IOCP we'll be using to receive messages for all jobs hasn't been
	// created, create it and start polling.
	if notifications {
		ioInitOnce.Do(func() {
			h, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0xffffffff)
			if err != nil {
				initIOErr = err
				return
			}
			ioCompletionPort = h
			go pollIOCP(ctx, h)
		})

		if initIOErr != nil {
			return nil, initIOErr
		}

		mq = queue.NewMessageQueue()
		jobMap.Store(uintptr(jobHandle), mq)
		if err = attachIOCP(jobHandle, ioCompletionPort); err != nil {
			jobMap.Delete(uintptr(jobHandle))
			return nil, err
		}
	}

	return &JobObject{
		handle: jobHandle,
		mq:     mq,
	}, nil
}

// SetResourceLimits sets resource limits on the job object (cpu, memory, storage).
func (job *JobObject) SetResourceLimits(limits *JobLimits) error {
	// Go through and check what limits were specified and apply them to the job.
	if limits.MemoryLimitInBytes != 0 {
		if err := job.SetMemoryLimit(limits.MemoryLimitInBytes); err != nil {
			return errors.Wrap(err, "failed to set job object memory limit")
		}
	}

	if limits.CPULimit != 0 {
		if err := job.SetCPULimit(RateBased, limits.CPULimit); err != nil {
			return errors.Wrap(err, "failed to set job object cpu limit")
		}
	} else if limits.CPUWeight != 0 {
		if err := job.SetCPULimit(WeightBased, limits.CPUWeight); err != nil {
			return errors.Wrap(err, "failed to set job object cpu limit")
		}
	}

	if limits.MaxBandwidth != 0 || limits.MaxIOPS != 0 {
		if err := job.SetIOLimit(limits.MaxBandwidth, limits.MaxIOPS); err != nil {
			return errors.Wrap(err, "failed to set io limit on job object")
		}
	}
	return nil
}

// SetCPULimit sets the CPU limit specified on the job object.
func (job *JobObject) SetCPULimit(rateControlType CPURateControlType, rateControlValue uint32) error {
	job.handleLock.RLock()
	defer job.handleLock.RUnlock()

	if job.handle == 0 {
		return ErrAlreadyClosed
	}

	var cpuInfo winapi.JOBOBJECT_CPU_RATE_CONTROL_INFORMATION
	switch rateControlType {
	case WeightBased:
		if rateControlValue < CPUWeightMin || rateControlValue > CPUWeightMax {
			return fmt.Errorf("processor weight value of `%d` is invalid", rateControlValue)
		}
		cpuInfo.ControlFlags = winapi.JOB_OBJECT_CPU_RATE_CONTROL_ENABLE | winapi.JOB_OBJECT_CPU_RATE_CONTROL_WEIGHT_BASED
		cpuInfo.Value = rateControlValue
	case RateBased:
		if rateControlValue < CPULimitMin || rateControlValue > CPULimitMax {
			return fmt.Errorf("processor rate of `%d` is invalid", rateControlValue)
		}
		cpuInfo.ControlFlags = winapi.JOB_OBJECT_CPU_RATE_CONTROL_ENABLE | winapi.JOB_OBJECT_CPU_RATE_CONTROL_HARD_CAP
		cpuInfo.Value = rateControlValue
	default:
		return errors.New("invalid job object cpu rate control type")
	}

	_, err := windows.SetInformationJobObject(job.handle, windows.JobObjectCpuRateControlInformation, uintptr(unsafe.Pointer(&cpuInfo)), uint32(unsafe.Sizeof(cpuInfo)))
	if err != nil {
		return fmt.Errorf("failed to set cpu limit info on job object: %s", err)
	}
	return nil
}

// SetMemoryLimit sets the memory limit specified on the job object.
func (job *JobObject) SetMemoryLimit(memoryLimitInBytes uint64) error {
	job.handleLock.RLock()
	defer job.handleLock.RUnlock()

	if job.handle == 0 {
		return ErrAlreadyClosed
	}

	var eliInfo windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	eliInfo.JobMemoryLimit = uintptr(memoryLimitInBytes)
	eliInfo.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_JOB_MEMORY
	_, err := windows.SetInformationJobObject(job.handle, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&eliInfo)), uint32(unsafe.Sizeof(eliInfo)))
	if err != nil {
		return fmt.Errorf("failed to set extended limit info on job object: %s", err)
	}
	return nil
}

// SetIOLimit sets the IO limits specified on the job object.
func (job *JobObject) SetIOLimit(maxBandwidth, maxIOPS int64) error {
	job.handleLock.RLock()
	defer job.handleLock.RUnlock()

	if job.handle == 0 {
		return ErrAlreadyClosed
	}

	ioInfo := winapi.JOBOBJECT_IO_RATE_CONTROL_INFORMATION{
		ControlFlags: winapi.JOB_OBJECT_IO_RATE_CONTROL_ENABLE,
	}
	if maxBandwidth != 0 {
		ioInfo.MaxBandwidth = maxBandwidth
	}
	if maxIOPS != 0 {
		ioInfo.MaxIops = maxIOPS
	}
	_, err := winapi.SetIoRateControlInformationJobObject(job.handle, &ioInfo)
	if err != nil {
		return fmt.Errorf("failed to set IO limit info on job object: %s", err)
	}
	return nil
}

// PollNotification will poll for a job object notification. This call should only be called once
// per job (ideally in a goroutine loop) and will block if there is not a notification ready.
// This call will return immediately with error `ErrNotRegistered` if the job was not registered
// to receive notifications during `Create`. Internally, messages will be queued and there
// is no worry of messages being dropped.
func (job *JobObject) PollNotification() (interface{}, error) {
	if job.mq == nil {
		return nil, ErrNotRegistered
	}
	return job.mq.ReadOrWait()
}

// Close closes the job object handle.
func (job *JobObject) Close() error {
	job.handleLock.Lock()
	defer job.handleLock.Unlock()

	if job.handle == 0 {
		return ErrAlreadyClosed
	}

	if err := windows.Close(job.handle); err != nil {
		return err
	}

	if job.mq != nil {
		job.mq.Close()
	}
	// Handles now invalid so if the map entry to receive notifications for this job still
	// exists remove it so we can stop receiving notifications.
	if _, ok := jobMap.Load(uintptr(job.handle)); ok {
		jobMap.Delete(uintptr(job.handle))
	}

	job.handle = 0
	return nil
}

// Assign assigns a process to the job object.
func (job *JobObject) Assign(pid uint32) error {
	job.handleLock.RLock()
	defer job.handleLock.RUnlock()

	if job.handle == 0 {
		return ErrAlreadyClosed
	}

	if pid == 0 {
		return errors.New("invalid pid: 0")
	}
	hProc, err := windows.OpenProcess(winapi.PROCESS_ALL_ACCESS, true, pid)
	if err != nil {
		return err
	}
	defer windows.Close(hProc)
	return windows.AssignProcessToJobObject(job.handle, hProc)
}

// Terminate terminates the job, essentially calls TerminateProcess on every process in the
// job.
func (job *JobObject) Terminate(exitCode uint32) error {
	job.handleLock.RLock()
	defer job.handleLock.RUnlock()
	if job.handle == 0 {
		return ErrAlreadyClosed
	}
	return windows.TerminateJobObject(job.handle, exitCode)
}

// Pids returns all of the process IDs in the job object.
func (job *JobObject) Pids() ([]uint32, error) {
	job.handleLock.RLock()
	defer job.handleLock.RUnlock()

	if job.handle == 0 {
		return nil, ErrAlreadyClosed
	}

	info := winapi.JOBOBJECT_BASIC_PROCESS_ID_LIST{}
	err := winapi.QueryInformationJobObject(
		job.handle,
		winapi.JobObjectBasicProcessIdList,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
		nil,
	)

	// This is either the case where there is only one process or no processes in
	// the job. Any other case will result in ERROR_MORE_DATA. Check if info.NumberOfProcessIdsInList
	// is 1 and just return this, otherwise return an empty slice.
	if err == nil {
		if info.NumberOfProcessIdsInList == 1 {
			return []uint32{uint32(info.ProcessIdList[0])}, nil
		}
		// Return empty slice instead of nil to play well with the caller of this.
		// Do not return an error if no processes are running inside the job
		return []uint32{}, nil
	}

	if err != winapi.ERROR_MORE_DATA {
		return nil, fmt.Errorf("failed initial query for PIDs in job object: %s", err)
	}

	jobBasicProcessIDListSize := unsafe.Sizeof(info) + (unsafe.Sizeof(info.ProcessIdList[0]) * uintptr(info.NumberOfAssignedProcesses-1))
	buf := make([]byte, jobBasicProcessIDListSize)
	if err = winapi.QueryInformationJobObject(
		job.handle,
		winapi.JobObjectBasicProcessIdList,
		uintptr(unsafe.Pointer(&buf[0])),
		uint32(len(buf)),
		nil,
	); err != nil {
		return nil, fmt.Errorf("failed to query for PIDs in job object: %s", err)
	}

	bufInfo := (*winapi.JOBOBJECT_BASIC_PROCESS_ID_LIST)(unsafe.Pointer(&buf[0]))
	bufPids := bufInfo.AllPids()
	pids := make([]uint32, bufInfo.NumberOfProcessIdsInList)
	for i, bufPid := range bufPids {
		pids[i] = uint32(bufPid)
	}
	return pids, nil
}

// QueryMemoryStats gets the memory stats for the job object.
func (job *JobObject) QueryMemoryStats() (*winapi.JOBOBJECT_MEMORY_USAGE_INFORMATION, error) {
	job.handleLock.RLock()
	defer job.handleLock.RUnlock()

	if job.handle == 0 {
		return nil, ErrAlreadyClosed
	}

	info := winapi.JOBOBJECT_MEMORY_USAGE_INFORMATION{}
	if err := winapi.QueryInformationJobObject(
		job.handle,
		winapi.JobObjectMemoryUsageInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
		nil,
	); err != nil {
		return nil, fmt.Errorf("failed to query for job object memory stats: %s", err)
	}
	return &info, nil
}

// QueryProcessorStats gets the processor stats for the job object.
func (job *JobObject) QueryProcessorStats() (*winapi.JOBOBJECT_BASIC_ACCOUNTING_INFORMATION, error) {
	job.handleLock.RLock()
	defer job.handleLock.RUnlock()

	if job.handle == 0 {
		return nil, ErrAlreadyClosed
	}

	info := winapi.JOBOBJECT_BASIC_ACCOUNTING_INFORMATION{}
	if err := winapi.QueryInformationJobObject(
		job.handle,
		winapi.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
		nil,
	); err != nil {
		return nil, fmt.Errorf("failed to query for job object process stats: %s", err)
	}
	return &info, nil
}

// QueryStorageStats gets the storage (I/O) stats for the job object.
func (job *JobObject) QueryStorageStats() (*winapi.JOBOBJECT_BASIC_AND_IO_ACCOUNTING_INFORMATION, error) {
	job.handleLock.RLock()
	defer job.handleLock.RUnlock()

	if job.handle == 0 {
		return nil, ErrAlreadyClosed
	}

	info := winapi.JOBOBJECT_BASIC_AND_IO_ACCOUNTING_INFORMATION{}
	if err := winapi.QueryInformationJobObject(
		job.handle,
		winapi.JobObjectBasicAndIoAccountingInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
		nil,
	); err != nil {
		return nil, fmt.Errorf("failed to query for job object storage stats: %s", err)
	}
	return &info, nil
}
