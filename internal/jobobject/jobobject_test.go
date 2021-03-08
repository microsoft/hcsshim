package jobobject

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

func TestJobNilOptions(t *testing.T) {
	_, err := Create(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestJobCreateAndOpen(t *testing.T) {
	var (
		ctx     = context.Background()
		options = &Options{Name: "test"}
	)

	jobCreate, err := Create(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer jobCreate.Close()

	jobOpen, err := Open(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer jobOpen.Close()
}

func createProcsAndAssign(num int, job *JobObject) (_ []*exec.Cmd, err error) {
	var procs []*exec.Cmd

	defer func() {
		if err != nil {
			for _, proc := range procs {
				proc.Process.Kill()
			}
		}
	}()

	for i := 0; i < num; i++ {
		cmd := exec.Command("ping", "-t", "127.0.0.1")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
		}

		if err := cmd.Start(); err != nil {
			return nil, err
		}

		if err := job.Assign(uint32(cmd.Process.Pid)); err != nil {
			return nil, err
		}
		procs = append(procs, cmd)
	}
	return procs, nil
}

func TestSetTerminateOnLastHandleClose(t *testing.T) {
	options := &Options{
		Name:          "test",
		Notifications: true,
	}
	job, err := Create(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer job.Close()

	if err := job.SetTerminateOnLastHandleClose(); err != nil {
		t.Fatal(err)
	}

	procs, err := createProcsAndAssign(1, job)
	if err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error)
	go func() {
		if err := job.Close(); err != nil {
			errCh <- err
		}
		if err := procs[0].Wait(); err != nil {
			errCh <- err
		}
		// Check if process is still alive after job handle close (it should not be).
		// If wait returned it should be gone but just to be explicit check anyways.
		if !procs[0].ProcessState.Exited() {
			errCh <- errors.New("process should have exited after closing job handle")
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second * 10):
		procs[0].Process.Kill()
		t.Fatal("process didn't complete wait within timeout")
	}
}

func TestSetMultipleExtendedLimits(t *testing.T) {
	// Tests setting two different properties on the job that modify
	// JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	options := &Options{
		Name:          "test",
		Notifications: true,
	}
	job, err := Create(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer job.Close()

	// No reason for this limit in particular. Could be any value.
	memLimitInMB := uint64(10 * 1024 * 1204)
	if err := job.SetMemoryLimit(memLimitInMB); err != nil {
		t.Fatal(err)
	}

	if err := job.SetTerminateOnLastHandleClose(); err != nil {
		t.Fatal(err)
	}

	eli, err := job.getExtendedInformation()
	if err != nil {
		t.Fatal(err)
	}

	if !isFlagSet(windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, eli.BasicLimitInformation.LimitFlags) {
		t.Fatal("the job does not have cpu rate control enabled")
	}

	if !isFlagSet(windows.JOB_OBJECT_LIMIT_JOB_MEMORY, eli.BasicLimitInformation.LimitFlags) {
		t.Fatal("the job does not have cpu rate control enabled")
	}

	if eli.JobMemoryLimit != uintptr(memLimitInMB) {
		t.Fatal("job memory limit not persisted")
	}
}

func TestNoMoreProcessesMessageKill(t *testing.T) {
	// Test that we receive the no more processes in job message after killing all of
	// the processes in the job.
	options := &Options{
		Name:          "test",
		Notifications: true,
	}
	job, err := Create(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer job.Close()

	if err := job.SetTerminateOnLastHandleClose(); err != nil {
		t.Fatal(err)
	}

	procs, err := createProcsAndAssign(2, job)
	if err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error)
	go func() {
		for _, proc := range procs {
			if err := proc.Process.Kill(); err != nil {
				errCh <- err
			}
		}

		for {
			notif, err := job.PollNotification()
			if err != nil {
				errCh <- err
			}

			switch notif.(type) {
			case MsgAllProcessesExited:
				errCh <- nil
			case MsgUnimplemented:
			default:
			}
		}
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second * 10):
		t.Fatal("didn't receive no more processes message within timeout")
	}
}

func TestNoMoreProcessesMessageTerminate(t *testing.T) {
	// Test that we receive the no more processes in job message after terminating the
	// job (terminates every process in the job).
	options := &Options{
		Name:          "test",
		Notifications: true,
	}
	job, err := Create(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer job.Close()

	if err := job.SetTerminateOnLastHandleClose(); err != nil {
		t.Fatal(err)
	}

	_, err = createProcsAndAssign(2, job)
	if err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error)
	go func() {
		if err := job.Terminate(1); err != nil {
			errCh <- err
		}

		for {
			notif, err := job.PollNotification()
			if err != nil {
				errCh <- err
			}

			switch notif.(type) {
			case MsgAllProcessesExited:
				errCh <- nil
			case MsgUnimplemented:
			default:
			}
		}
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second * 10):
		t.Fatal("didn't receive no more processes message within timeout")
	}
}
