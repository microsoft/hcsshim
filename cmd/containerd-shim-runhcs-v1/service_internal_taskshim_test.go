package main

import (
	"context"
	"testing"

	"github.com/containerd/containerd/errdefs"
	runcopts "github.com/containerd/containerd/runtime/v2/runc/options"
	"github.com/containerd/containerd/runtime/v2/task"
	"github.com/containerd/typeurl"
)

func setupTaskServiceWithFakes(t *testing.T) (*service, *testShimTask) {
	s := service{
		tid:       t.Name(),
		isSandbox: false,
	}
	task := &testShimTask{
		id: t.Name(),
		exec: &testShimExec{
			id:  "", // Fake init pid ID
			pid: 10,
		},
		execs: map[string]*testShimExec{
			t.Name() + "-2": {
				id:  t.Name() + "-2", // Fake 2nd pid ID
				pid: 101,
			},
		},
	}
	s.taskOrPod.Store(task)
	return &s, task
}

func Test_TaskShim_getTask_NotCreated_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: false,
	}

	st, err := s.getTask(t.Name())

	verifyExpectedError(t, st, err, errdefs.ErrNotFound)
}

func Test_TaskShim_getTask_Created_DifferentID_Error(t *testing.T) {
	s, _ := setupTaskServiceWithFakes(t)

	st, err := s.getTask("thisidwontmatch")

	verifyExpectedError(t, st, err, errdefs.ErrNotFound)
}

func Test_TaskShim_getTask_Created_CorrectID_Success(t *testing.T) {
	s, t1 := setupTaskServiceWithFakes(t)

	st, err := s.getTask(t.Name())
	if err != nil {
		t.Fatalf("should have not failed with error, got: %v", err)
	}
	if st != t1 {
		t.Fatal("should of returned a valid task")
	}
}

func Test_TaskShim_stateInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: false,
	}

	resp, err := s.stateInternal(context.TODO(), &task.StateRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_stateInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
	s, _ := setupTaskServiceWithFakes(t)

	resp, err := s.stateInternal(context.TODO(), &task.StateRequest{
		ID:     t.Name(),
		ExecID: "thisshouldnotmatch",
	})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_stateInternal_InitTaskID_InitExecID_Success(t *testing.T) {
	s, t1 := setupTaskServiceWithFakes(t)

	resp, err := s.stateInternal(context.TODO(), &task.StateRequest{
		ID:     t.Name(),
		ExecID: "",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned StateResponse")
	}
	if resp.ID != "" || resp.Pid != uint32(t1.exec.pid) {
		t.Fatalf("should of returned init pid, got: %v", resp)
	}
}

func Test_TaskShim_stateInternal_InitTaskID_2ndExecID_Success(t *testing.T) {
	s, t1 := setupTaskServiceWithFakes(t)

	eid := t.Name() + "-2"
	resp, err := s.stateInternal(context.TODO(), &task.StateRequest{
		ID:     t.Name(),
		ExecID: eid,
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned StateResponse")
	}
	if resp.ID != t1.execs[eid].id || resp.Pid != uint32(t1.execs[eid].pid) {
		t.Fatalf("should of returned 2nd exec pid, got: %v", resp)
	}
}

// TODO: Test_TaskShim_createInternal_*

func Test_TaskShim_startInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.startInternal(context.TODO(), &task.StartRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_startInternal_ValidTask_DifferentExecID_Error(t *testing.T) {
	s, _ := setupTaskServiceWithFakes(t)

	resp, err := s.startInternal(context.TODO(), &task.StartRequest{
		ID:     t.Name(),
		ExecID: "thisshouldnotmatch",
	})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_startInternal_InitTaskID_InitExecID_Success(t *testing.T) {
	s, t1 := setupTaskServiceWithFakes(t)

	resp, err := s.startInternal(context.TODO(), &task.StartRequest{
		ID:     t.Name(),
		ExecID: "",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned StartResponse")
	}
	if resp.Pid != uint32(t1.exec.pid) {
		t.Fatal("should of returned init pid")
	}
}

func Test_TaskShim_startInternal_InitTaskID_2ndExecID_Success(t *testing.T) {
	s, t1 := setupTaskServiceWithFakes(t)

	eid := t.Name() + "-2"
	resp, err := s.startInternal(context.TODO(), &task.StartRequest{
		ID:     t.Name(),
		ExecID: eid,
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned StartResponse")
	}
	if resp.Pid != uint32(t1.execs[eid].pid) {
		t.Fatal("should of returned 2nd pid")
	}
}

func Test_TaskShim_deleteInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.deleteInternal(context.TODO(), &task.DeleteRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_deleteInternal_ValidTask_DifferentExecID_Error(t *testing.T) {
	s, _ := setupTaskServiceWithFakes(t)

	resp, err := s.deleteInternal(context.TODO(), &task.DeleteRequest{
		ID:     t.Name(),
		ExecID: "thisshouldnotmatch",
	})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_deleteInternal_InitTaskID_InitExecID_Success(t *testing.T) {
	s, t1 := setupTaskServiceWithFakes(t)

	resp, err := s.deleteInternal(context.TODO(), &task.DeleteRequest{
		ID:     t.Name(),
		ExecID: "",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned DeleteResponse")
	}
	if resp.Pid != uint32(t1.exec.pid) {
		t.Fatal("should of returned init pid")
	}
}

func Test_TaskShim_deleteInternal_InitTaskID_2ndExecID_Success(t *testing.T) {
	s, t1 := setupTaskServiceWithFakes(t)

	// capture the t2 task as it will be deleted
	eid := t.Name() + "-2"
	t2t := t1.execs[eid]

	resp, err := s.deleteInternal(context.TODO(), &task.DeleteRequest{
		ID:     t.Name(),
		ExecID: eid,
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned DeleteResponse")
	}
	if resp.Pid != uint32(t2t.pid) {
		t.Fatal("should of returned 2nd pid")
	}
	if _, ok := t1.execs[eid]; ok {
		t.Fatal("should of deleted the 2nd exec")
	}
}

func Test_TaskShim_pidsInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.pidsInternal(context.TODO(), &task.PidsRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_pidsInternal_InitTaskID_Success(t *testing.T) {
	s, t1 := setupTaskServiceWithFakes(t)

	eid := t.Name() + "-2"
	resp, err := s.pidsInternal(context.TODO(), &task.PidsRequest{ID: t.Name()})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned PidsResponse")
	}
	if len(resp.Processes) != 2 {
		t.Fatalf("should of returned len(processes) == 1, got: %v", len(resp.Processes))
	}
	if resp.Processes[0].Pid != uint32(t1.exec.pid) {
		t.Fatal("should of returned init pid")
	}
	if resp.Processes[0].Info != nil {
		t.Fatal("should of returned nil init pid info")
	}
	if resp.Processes[1].Pid != uint32(t1.execs[eid].pid) {
		t.Fatal("should of returned 2nd pid")
	}
	if resp.Processes[1].Info == nil {
		t.Fatal("should not have returned nil 2nd pid info")
	}
	u, err := typeurl.UnmarshalAny(resp.Processes[1].Info)
	if err != nil {
		t.Fatalf("failed to unmarshal 2nd pid info, err: %v", err)
	}
	pi := u.(*runcopts.ProcessDetails)
	if pi.ExecID != eid {
		t.Fatalf("should of returned 2nd pid ExecID, got: %v", pi.ExecID)
	}
}

func Test_TaskShim_pauseInternal_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.pauseInternal(context.TODO(), &task.PauseRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
}

func Test_TaskShim_resumeInternal_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.resumeInternal(context.TODO(), &task.ResumeRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
}

func Test_TaskShim_checkpointInternal_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.checkpointInternal(context.TODO(), &task.CheckpointTaskRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
}

func Test_TaskShim_killInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.killInternal(context.TODO(), &task.KillRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_killInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
	s, _ := setupTaskServiceWithFakes(t)

	resp, err := s.killInternal(context.TODO(), &task.KillRequest{
		ID:     t.Name(),
		ExecID: "thisshouldnotmatch",
	})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_killInternal_InitTaskID_InitExecID_Success(t *testing.T) {
	s, _ := setupTaskServiceWithFakes(t)

	resp, err := s.killInternal(context.TODO(), &task.KillRequest{
		ID:     t.Name(),
		ExecID: "",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned KillResponse")
	}
}

func Test_TaskShim_killInternal_InitTaskID_2ndExecID_Success(t *testing.T) {
	s, _ := setupTaskServiceWithFakes(t)

	resp, err := s.killInternal(context.TODO(), &task.KillRequest{
		ID:     t.Name(),
		ExecID: t.Name() + "-2",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned KillResponse")
	}
}

// TODO: Test_TaskShim_execInternal_*

func Test_TaskShim_resizePtyInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.resizePtyInternal(context.TODO(), &task.ResizePtyRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_resizePtyInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
	s, _ := setupTaskServiceWithFakes(t)

	resp, err := s.resizePtyInternal(context.TODO(), &task.ResizePtyRequest{
		ID:     t.Name(),
		ExecID: "thisshouldnotmatch",
	})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_resizePtyInternal_InitTaskID_InitExecID_Success(t *testing.T) {
	s, _ := setupTaskServiceWithFakes(t)

	resp, err := s.resizePtyInternal(context.TODO(), &task.ResizePtyRequest{
		ID:     t.Name(),
		ExecID: "",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned ResizePtyResponse")
	}
}

func Test_TaskShim_resizePtyInternal_InitTaskID_2ndExecID_Success(t *testing.T) {
	s, _ := setupTaskServiceWithFakes(t)

	resp, err := s.resizePtyInternal(context.TODO(), &task.ResizePtyRequest{
		ID:     t.Name(),
		ExecID: t.Name() + "-2",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned ResizePtyResponse")
	}
}

func Test_TaskShim_closeIOInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.closeIOInternal(context.TODO(), &task.CloseIORequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_closeIOInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
	s, _ := setupTaskServiceWithFakes(t)

	resp, err := s.closeIOInternal(context.TODO(), &task.CloseIORequest{
		ID:     t.Name(),
		ExecID: "thisshouldnotmatch",
	})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_closeIOInternal_InitTaskID_InitExecID_Success(t *testing.T) {
	s, _ := setupTaskServiceWithFakes(t)

	resp, err := s.closeIOInternal(context.TODO(), &task.CloseIORequest{
		ID:     t.Name(),
		ExecID: "",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned CloseIOResponse")
	}
}

func Test_TaskShim_closeIOInternal_InitTaskID_2ndExecID_Success(t *testing.T) {
	s, _ := setupTaskServiceWithFakes(t)

	resp, err := s.closeIOInternal(context.TODO(), &task.CloseIORequest{
		ID:     t.Name(),
		ExecID: t.Name() + "-2",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned CloseIOResponse")
	}
}

func Test_TaskShim_updateInternal_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.updateInternal(context.TODO(), &task.UpdateTaskRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
}

func Test_TaskShim_waitInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.waitInternal(context.TODO(), &task.WaitRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_waitInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
	s, _ := setupTaskServiceWithFakes(t)

	resp, err := s.waitInternal(context.TODO(), &task.WaitRequest{
		ID:     t.Name(),
		ExecID: "thisshouldnotmatch",
	})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_TaskShim_waitInternal_InitTaskID_InitExecID_Success(t *testing.T) {
	s, t1 := setupTaskServiceWithFakes(t)

	resp, err := s.waitInternal(context.TODO(), &task.WaitRequest{
		ID:     t.Name(),
		ExecID: "",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned WaitResponse")
	}
	if resp.ExitStatus != t1.exec.Status().ExitStatus {
		t.Fatal("should of returned exit status for init")
	}
}

func Test_TaskShim_waitInternal_InitTaskID_2ndExecID_Success(t *testing.T) {
	s, t1 := setupTaskServiceWithFakes(t)

	eid := t.Name() + "-2"
	resp, err := s.waitInternal(context.TODO(), &task.WaitRequest{
		ID:     t.Name(),
		ExecID: eid,
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned WaitResponse")
	}
	if resp.ExitStatus != t1.execs[eid].Status().ExitStatus {
		t.Fatal("should of returned exit status for init")
	}
}

func Test_TaskShim_statsInternal_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.statsInternal(context.TODO(), &task.StatsRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
}
