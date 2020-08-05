package privileged

import (
	"context"
	"errors"
	"fmt"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"golang.org/x/sys/windows"
)

// This file provides higher level constructs for the win32 job object API.
// Most of the core creation and management functions are already present in "golang.org/x/sys/windows"
// (CreateJobObject, AssignProcessToJobObject, etc.) as well as most of the limit information
// structs and associated limit flags. Whatever is not present from the job object API
// in golang.org/x/sys/windows is located in /internal/winapi.
//
// https://docs.microsoft.com/en-us/windows/win32/procthread/job-objects

// jobObject is a high level wrapper around a Windows job object. Holds a handle to
// the job and a handle to an iocp
type jobObject struct {
	jobHandle  windows.Handle
	iocpHandle windows.Handle
}

type jobLimits struct {
	// cpu count
	affinity       uintptr
	cpuRate        uint32
	cpuWeight      uint32
	jobMemoryLimit uintptr
	maxIops        int64
	maxBandwidth   int64
}

func (job *jobObject) setResourceLimits(ctx context.Context, limits *jobLimits) error {
	// Go through and check what limits were specified and construct the appropriate
	// structs.
	if limits.affinity != 0 || limits.jobMemoryLimit != 0 {
		var (
			basicLimitFlags uint32
			basicInfo       windows.JOBOBJECT_BASIC_LIMIT_INFORMATION
			eliInfo         windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		)
		eliInfo.BasicLimitInformation = basicInfo
		if limits.affinity != 0 {
			basicLimitFlags |= windows.JOB_OBJECT_LIMIT_AFFINITY
			eliInfo.BasicLimitInformation.Affinity = limits.affinity
		}
		if limits.jobMemoryLimit != 0 {
			basicLimitFlags |= windows.JOB_OBJECT_LIMIT_JOB_MEMORY
			eliInfo.JobMemoryLimit = limits.jobMemoryLimit
		}
		eliPtr := unsafe.Pointer(&eliInfo)
		_, err := windows.SetInformationJobObject(job.jobHandle, windows.JobObjectExtendedLimitInformation, uintptr(eliPtr), uint32(unsafe.Sizeof(eliInfo)))
		if err != nil {
			return fmt.Errorf("failed to set extended limit info on job object: %s", err)
		}
	}

	if limits.cpuRate != 0 {
		cpuInfo := winapi.JOBOBJECT_CPU_RATE_CONTROL_INFORMATION{
			ControlFlags: winapi.JOB_OBJECT_CPU_RATE_CONTROL_ENABLE | winapi.JOB_OBJECT_CPU_RATE_CONTROL_HARD_CAP,
			Rate:         limits.cpuRate,
		}
		cpuPtr := unsafe.Pointer(&cpuInfo)
		_, err := windows.SetInformationJobObject(job.jobHandle, windows.JobObjectCpuRateControlInformation, uintptr(cpuPtr), uint32(unsafe.Sizeof(cpuInfo)))
		if err != nil {
			return fmt.Errorf("failed to set cpu limit info on job object: %s", err)
		}
	}

	if limits.maxBandwidth != 0 || limits.maxIops != 0 {
		ioInfo := winapi.JOBOBJECT_IO_RATE_CONTROL_INFORMATION{
			ControlFlags: winapi.JOB_OBJECT_IO_RATE_CONTROL_ENABLE,
		}
		if limits.maxBandwidth != 0 {
			ioInfo.MaxBandwidth = limits.maxBandwidth
		}
		if limits.maxIops != 0 {
			ioInfo.MaxIops = limits.maxIops
		}
		_, err := winapi.SetIoRateControlInformationJobObject(job.jobHandle, &ioInfo)
		if err != nil {
			return fmt.Errorf("failed to set IO limit info on job object: %s", err)
		}
	}
	return nil
}

// CreateJobObject creates a job object, attaches an IO completion port to use
// for notifications and then returns an object with the corresponding handles.
func createJobObject(name string) (*jobObject, error) {
	jobHandle, err := windows.CreateJobObject(nil, windows.StringToUTF16Ptr(name))
	if err != nil {
		return nil, err
	}
	iocpHandle, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 1)
	if err != nil {
		windows.Close(jobHandle)
		return nil, err
	}
	if _, err = attachIOCP(jobHandle, iocpHandle); err != nil {
		windows.Close(jobHandle)
		windows.Close(iocpHandle)
		return nil, err
	}
	return &jobObject{jobHandle, iocpHandle}, nil
}

// Close closes the job object and iocp handles. If this is the only open handle
// the job object will be terminated.
func (job *jobObject) close() error {
	closeErr := false
	if job.jobHandle != 0 {
		if err := windows.Close(job.jobHandle); err != nil {
			closeErr = true
		}
	}
	if job.iocpHandle != 0 {
		if err := windows.Close(job.iocpHandle); err != nil {
			closeErr = true
		}
	}
	if closeErr {
		return errors.New("failed to close one or more handles")
	}
	return nil
}

// Assign assigns a process to the job object.
func (job *jobObject) assign(p *JobProcess) error {
	if p.Pid() == 0 {
		return errors.New("process has not started")
	}
	hProc, err := windows.OpenProcess(winapi.PROCESS_ALL_ACCESS, true, uint32(p.Pid()))
	if err != nil {
		return err
	}
	defer windows.Close(hProc)
	return windows.AssignProcessToJobObject(job.jobHandle, hProc)
}

// Terminates the job, essentially calls TerminateProcess on every process in the
// job.
func (job *jobObject) terminate() error {
	if job.jobHandle != 0 {
		return windows.TerminateJobObject(job.jobHandle, 1)
	}
	return nil
}

func (job *jobObject) shutdown(ctx context.Context) error {
	pids, err := job.pids()
	if err != nil {
		return fmt.Errorf("failed to get pids for job object: %s", err)
	}
	var (
		terminate bool
		signalErr bool
	)
	for _, pid := range pids {
		if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, pid); err != nil {
			log.G(ctx).WithField("pid", pid).Error("failed to send ctrl-break to process in job")
			signalErr = true
		}
	}

	// Get pids in job again. If there is any left then terminate the job.
	newPids, err := job.pids()
	if err != nil {
		return fmt.Errorf("failed to get pids for job object: %s", err)
	}
	terminate = len(newPids) != 0
	// If any of the processes couldnt be killed gracefully just terminate the job.
	// Equivalent to calling TerminateProcess on every proc in the job.
	if terminate || signalErr {
		return job.terminate()
	}
	return nil
}

// Returns all of the process IDs in the job object.
func (job *jobObject) pids() ([]uint32, error) {
	info := winapi.JOBOBJECT_BASIC_PROCESS_ID_LIST{}
	err := winapi.QueryInformationJobObject(
		job.jobHandle,
		winapi.JobObjectBasicProcessIdList,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
		0)

	// If there are no processes in the job the next call to QueryInformation will just
	// hang until a memory alloc error. Return here.
	if err == nil {
		// Return empty slice instead of nil to play well with the caller of this.
		// I don't think we should return an error on there being no PIDs.
		var pids []uint32
		return pids, nil
	}

	if err != nil && err != winapi.ERROR_MORE_DATA {
		return nil, fmt.Errorf("failed initial query for PIDs in job object: %s", err)
	}

	buf := make([]uintptr, unsafe.Sizeof(info)+unsafe.Sizeof(info.ProcessIdList[0])*uintptr(info.NumberOfAssignedProcesses-1))
	err = winapi.QueryInformationJobObject(
		job.jobHandle,
		winapi.JobObjectBasicProcessIdList,
		uintptr(unsafe.Pointer(&buf[0])),
		uint32(len(buf)),
		0)

	if err != nil {
		return nil, fmt.Errorf("failed to query for PIDs in job object: %s", err)
	}

	bufInfo := (*winapi.JOBOBJECT_BASIC_PROCESS_ID_LIST)(unsafe.Pointer(&buf[0]))
	rawPids := make([]uintptr, bufInfo.NumberOfProcessIdsInList)

	err = winapi.RtlMoveMemory(
		(*byte)(unsafe.Pointer(&rawPids[0])),
		(*byte)(unsafe.Pointer(&bufInfo.ProcessIdList[0])),
		uintptr(bufInfo.NumberOfProcessIdsInList)*unsafe.Sizeof(rawPids[0]))
	if err != nil {
		return nil, fmt.Errorf("failed to move PID info to new buffer: %s", err)
	}

	pids := make([]uint32, bufInfo.NumberOfProcessIdsInList)
	for i, rawPid := range rawPids {
		pids[i] = uint32(rawPid)
	}
	return pids, nil
}

// Polls the IO completion port for notifications. Used for detecting when all of the
// processes in a job have exited and for (TODO: dcantah) limit thresholds being reached.
func (job *jobObject) pollIOCP() (uint32, error) {
	var (
		overlapped uintptr
		qty        uint32
		key        uint32
	)
	if job.iocpHandle != 0 {
		if err := windows.GetQueuedCompletionStatus(job.iocpHandle, &qty, &key, (**windows.Overlapped)(unsafe.Pointer(&overlapped)), windows.INFINITE); err != nil {
			return 0, err
		}
		return qty, nil
	}
	return 0, errors.New("IOCP handle is 0")
}

// Assigns an IO completion port to get notified of events for the registered job
// object.
func attachIOCP(job windows.Handle, iocp windows.Handle) (int, error) {
	info := winapi.JOBOBJECT_ASSOCIATE_COMPLETION_PORT{
		CompletionKey:  uintptr(job),
		CompletionPort: iocp,
	}
	infoPtr := unsafe.Pointer(&info)
	// JobObjectAssociateCompletionPortInformation
	// https://docs.microsoft.com/en-us/windows/win32/api/jobapi2/nf-jobapi2-setinformationjobobject
	return windows.SetInformationJobObject(job, windows.JobObjectAssociateCompletionPortInformation, uintptr(infoPtr), uint32(unsafe.Sizeof(info)))
}
