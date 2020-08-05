package privileged

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/winapi"
	"golang.org/x/sys/windows"
)

// Helper to create processes to assign to job object tests
func createProcesses(count int) []*JobProcess {
	procs := make([]*JobProcess, count)
	for i := 0; i < count; i++ {
		cmd := exec.Command("ping", "-t", "127.0.0.1")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
		}
		procs[i] = newProcess(cmd)
	}
	return procs
}

func startAndWait(job *jobObject, procs []*JobProcess) error {
	for _, proc := range procs {
		if err := proc.start(); err != nil {
			return err
		}
		if err := job.assign(proc); err != nil {
			return err
		}
		go proc.waitBackground(context.Background())
	}
	return nil
}

func cleanup(job *jobObject, procs []*JobProcess) {
	job.close()
	for _, proc := range procs {
		if code, _ := proc.ExitCode(); code == -1 {
			proc.Kill(context.Background())
		}
	}
}

func TestCreateAndTerminateJob(t *testing.T) {
	job, err := createJobObject("test")
	if err != nil {
		t.Fatalf("failed to create job object: %s", err)
	}

	procs := createProcesses(2)

	defer func() {
		cleanup(job, procs)
	}()

	if err := startAndWait(job, procs); err != nil {
		t.Fatalf("failed to start and wait on processes")
	}

	if err := job.terminate(); err != nil {
		t.Fatalf("failed to terminate job object: %s", err)
	}
}

func TestCreateAndShutdownJob(t *testing.T) {
	job, err := createJobObject("test")
	if err != nil {
		t.Fatalf("failed to create job object: %s", err)
	}

	procs := createProcesses(2)

	defer func() {
		cleanup(job, procs)
	}()

	if err := startAndWait(job, procs); err != nil {
		t.Fatalf("failed to start and wait on processes")
	}

	if err := job.shutdown(context.Background()); err != nil {
		t.Fatalf("failed to shutdown job object: %s", err)
	}
}

func TestCreateJobSetLimits(t *testing.T) {
	job, err := createJobObject("test")
	if err != nil {
		t.Fatalf("failed to create job object: %s", err)
	}

	limits := &jobLimits{
		affinity:       uintptr(int32ToBitmask(int32(runtime.NumCPU()))),
		cpuRate:        100 * 90,
		jobMemoryLimit: 100 * 1024 * 1024,
		maxBandwidth:   1000,
		maxIops:        1000,
	}
	if err := job.setResourceLimits(context.Background(), limits); err != nil {
		job.close()
		t.Fatalf("failed to set resource limits on job: %s", err)
	}

	procs := createProcesses(2)

	defer func() {
		cleanup(job, procs)
	}()

	if err := startAndWait(job, procs); err != nil {
		t.Fatalf("failed to start and wait on processes")
	}

	if err := job.shutdown(context.Background()); err != nil {
		t.Fatalf("failed to shutdown job: %s", err)
	}
}

func TestJobPIDs(t *testing.T) {
	job, err := createJobObject("test")
	if err != nil {
		t.Fatalf("failed to create job object: %s", err)
	}

	procs := createProcesses(2)

	defer func() {
		cleanup(job, procs)
	}()

	if err := startAndWait(job, procs); err != nil {
		t.Fatalf("failed to start and wait on processes")
	}

	pidsMap := make(map[int]struct{})
	for _, proc := range procs {
		pidsMap[proc.Pid()] = struct{}{}
	}

	pids, err := job.pids()
	if err != nil {
		t.Fatalf("failed to get PIDs in job: %s", err)
	}

	if len(pids) != len(procs) {
		t.Fatalf("number of PIDs in job incorrect")
	}

	for i := 0; i < len(pids); i++ {
		if _, ok := pidsMap[int(pids[i])]; !ok {
			t.Fatalf("PID not present in job object")
		}
	}

	if err := job.shutdown(context.Background()); err != nil {
		t.Fatalf("failed to shutdown job: %s", err)
	}
}

func TestJobNotificationShutdown(t *testing.T) {
	job, err := createJobObject("test")
	if err != nil {
		t.Fatalf("failed to create job object: %s", err)
	}

	procs := createProcesses(2)

	defer func() {
		cleanup(job, procs)
	}()

	if err := startAndWait(job, procs); err != nil {
		t.Fatalf("failed to start and wait on processes")
	}

	go func() {
		time.Sleep(time.Second * 2)
		if err := job.shutdown(context.Background()); err != nil {
			fmt.Println(err)
		}
	}()

	for {
		code, err := job.pollIOCP()
		if err != nil {
			t.Fatalf("failed to poll IOCP: %s", err)
		}

		if code == winapi.JOB_OBJECT_MSG_ACTIVE_PROCESS_ZERO {
			return
		}
	}
}

func TestJobNotificationKill(t *testing.T) {
	job, err := createJobObject("test")
	if err != nil {
		t.Fatalf("failed to create job object: %s", err)
	}

	procs := createProcesses(2)

	defer func() {
		cleanup(job, procs)
	}()

	if err := startAndWait(job, procs); err != nil {
		t.Fatalf("failed to start and wait on processes")
	}

	go func() {
		time.Sleep(time.Second * 1)
		for _, proc := range procs {
			proc.Kill(context.Background())
		}
	}()

	for {
		code, err := job.pollIOCP()
		if err != nil {
			t.Fatalf("failed to poll IOCP: %s", err)
		}

		if code == winapi.JOB_OBJECT_MSG_ACTIVE_PROCESS_ZERO {
			return
		}
	}
}

func TestKillProcess(t *testing.T) {
	job, err := createJobObject("test")
	if err != nil {
		t.Fatalf("failed to create job object: %s", err)
	}

	procs := createProcesses(1)

	if err := startAndWait(job, procs); err != nil {
		t.Fatalf("failed to start and wait on processes")
	}

	defer func() {
		job.terminate()
		job.close()
	}()

	for _, proc := range procs {
		if code, _ := proc.ExitCode(); code == -1 {
			if _, err := proc.Kill(context.Background()); err != nil {
				t.Fatalf("failed to kill process: %s", err)
			}
		}
	}
}
