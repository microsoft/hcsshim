package main

import (
	"context"
	"testing"

	"github.com/containerd/containerd/errdefs"
	runcopts "github.com/containerd/containerd/runtime/v2/runc/options"
	"github.com/containerd/containerd/runtime/v2/task"
	"github.com/containerd/typeurl"
)

func setupPodServiceWithFakes(t *testing.T) (*service, *testShimTask, *testShimTask) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	pod := &testShimPod{id: t.Name()}

	// create init fake container
	task := &testShimTask{
		id:   t.Name(),
		exec: newTestShimExec(t.Name(), "", 10),
	}

	// create a 2nd fake container
	task2 := &testShimTask{
		id:   t.Name() + "-2",
		exec: newTestShimExec(t.Name()+"-2", "", 101),
		execs: map[string]*testShimExec{
			t.Name() + "-2": newTestShimExec(t.Name()+"-2", t.Name()+"-2", 201),
		},
	}

	// store the init task and 2nd task in the pod
	pod.tasks.Store(task.id, task)
	pod.tasks.Store(task2.id, task2)
	s.taskOrPod.Store(pod)
	return &s, task, task2
}

func Test_PodShim_getPod_NotCreated_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	p, err := s.getPod()

	verifyExpectedError(t, p, err, errdefs.ErrFailedPrecondition)
}

func Test_PodShim_getPod_Created_Success(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

	p, err := s.getPod()
	if err != nil {
		t.Fatalf("should have not failed with error, got: %v", err)
	}
	if p == nil {
		t.Fatal("should of returned a valid pod")
	}
}

func Test_PodShim_getTask_NotCreated_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	st, err := s.getTask(t.Name())

	verifyExpectedError(t, st, err, errdefs.ErrNotFound)
}

func Test_PodShim_getTask_Created_DifferentID_Error(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

	st, err := s.getTask("thisidwontmatch")

	verifyExpectedError(t, st, err, errdefs.ErrNotFound)
}

func Test_PodShim_getTask_Created_InitID_Success(t *testing.T) {
	s, t1, _ := setupPodServiceWithFakes(t)

	st, err := s.getTask(t.Name())
	if err != nil {
		t.Fatalf("should have not failed with error, got: %v", err)
	}
	if st != t1 {
		t.Fatal("should of returned a valid task")
	}
}

func Test_PodShim_getTask_Created_2ndID_Success(t *testing.T) {
	s, _, t2 := setupPodServiceWithFakes(t)

	st, err := s.getTask(t.Name() + "-2")
	if err != nil {
		t.Fatalf("should have not failed with error, got: %v", err)
	}
	if st != t2 {
		t.Fatal("should of returned a valid task")
	}
}

func Test_PodShim_stateInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.stateInternal(context.TODO(), &task.StateRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_stateInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

	resp, err := s.stateInternal(context.TODO(), &task.StateRequest{
		ID:     t.Name(),
		ExecID: "thisshouldnotmatch",
	})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_stateInternal_InitTaskID_InitExecID_Success(t *testing.T) {
	s, t1, _ := setupPodServiceWithFakes(t)

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
	if resp.ID != t1.ID() || resp.Pid != uint32(t1.exec.pid) {
		t.Fatalf("should of returned init pid, got: %v", resp)
	}
}

func Test_PodShim_stateInternal_2ndTaskID_2ndExecID_Success(t *testing.T) {
	s, _, t2 := setupPodServiceWithFakes(t)

	resp, err := s.stateInternal(context.TODO(), &task.StateRequest{
		ID:     t.Name() + "-2",
		ExecID: t.Name() + "-2",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned StateResponse")
	}
	if resp.ID != t2.ID() || resp.Pid != uint32(t2.execs[t2.id].pid) {
		t.Fatal("should of returned 2nd pid")
	}
}

// TODO: Test_PodShim_createInternal_*

func Test_PodShim_startInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.startInternal(context.TODO(), &task.StartRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_startInternal_ValidTask_DifferentExecID_Error(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

	resp, err := s.startInternal(context.TODO(), &task.StartRequest{
		ID:     t.Name(),
		ExecID: "thisshouldnotmatch",
	})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_startInternal_InitTaskID_InitExecID_Success(t *testing.T) {
	s, t1, _ := setupPodServiceWithFakes(t)

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

func Test_PodShim_startInternal_2ndTaskID_2ndExecID_Success(t *testing.T) {
	s, _, t2 := setupPodServiceWithFakes(t)

	resp, err := s.startInternal(context.TODO(), &task.StartRequest{
		ID:     t.Name() + "-2",
		ExecID: t.Name() + "-2",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned StartResponse")
	}
	if resp.Pid != uint32(t2.execs[t2.id].pid) {
		t.Fatal("should of returned 2nd pid")
	}
}

func Test_PodShim_deleteInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.deleteInternal(context.TODO(), &task.DeleteRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_deleteInternal_ValidTask_DifferentExecID_Error(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

	resp, err := s.deleteInternal(context.TODO(), &task.DeleteRequest{
		ID:     t.Name(),
		ExecID: "thisshouldnotmatch",
	})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_deleteInternal_InitTaskID_InitExecID_Success(t *testing.T) {
	s, t1, _ := setupPodServiceWithFakes(t)

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

func Test_PodShim_deleteInternal_2ndTaskID_2ndExecID_Success(t *testing.T) {
	s, _, t2 := setupPodServiceWithFakes(t)

	// capture the t2 task as it will be deleted
	t2t := t2.execs[t2.id]

	resp, err := s.deleteInternal(context.TODO(), &task.DeleteRequest{
		ID:     t.Name() + "-2",
		ExecID: t.Name() + "-2",
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
	if _, ok := t2.execs[t2.id]; ok {
		t.Fatal("should of deleted the 2nd exec")
	}
}

func Test_PodShim_pidsInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.pidsInternal(context.TODO(), &task.PidsRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_pidsInternal_InitTaskID_Success(t *testing.T) {
	s, t1, _ := setupPodServiceWithFakes(t)

	resp, err := s.pidsInternal(context.TODO(), &task.PidsRequest{ID: t.Name()})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned PidsResponse")
	}
	if len(resp.Processes) != 1 {
		t.Fatalf("should of returned len(processes) == 1, got: %v", len(resp.Processes))
	}
	if resp.Processes[0].Pid != uint32(t1.exec.pid) {
		t.Fatal("should of returned init pid")
	}
	if resp.Processes[0].Info != nil {
		t.Fatal("should of returned nil init pid info")
	}
}

func Test_PodShim_pidsInternal_2ndTaskID_Success(t *testing.T) {
	s, _, t2 := setupPodServiceWithFakes(t)

	resp, err := s.pidsInternal(context.TODO(), &task.PidsRequest{ID: t.Name() + "-2"})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned PidsResponse")
	}
	if len(resp.Processes) != 2 {
		t.Fatalf("should of returned len(processes) == 2, got: %v", len(resp.Processes))
	}
	if resp.Processes[0].Pid != uint32(t2.exec.pid) {
		t.Fatal("should of returned init pid")
	}
	if resp.Processes[0].Info != nil {
		t.Fatal("should of returned nil init pid info")
	}
	if resp.Processes[1].Pid != uint32(t2.execs[t2.id].pid) {
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
	if pi.ExecID != t2.id {
		t.Fatalf("should of returned 2nd pid ExecID, got: %v", pi.ExecID)
	}
}

func Test_PodShim_pauseInternal_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.pauseInternal(context.TODO(), &task.PauseRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
}

func Test_PodShim_resumeInternal_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.resumeInternal(context.TODO(), &task.ResumeRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
}

func Test_PodShim_checkpointInternal_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.checkpointInternal(context.TODO(), &task.CheckpointTaskRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
}

func Test_PodShim_killInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.killInternal(context.TODO(), &task.KillRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_killInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

	resp, err := s.killInternal(context.TODO(), &task.KillRequest{
		ID:     t.Name(),
		ExecID: "thisshouldnotmatch",
	})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_killInternal_InitTaskID_InitExecID_Success(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

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

func Test_PodShim_killInternal_2ndTaskID_2ndExecID_Success(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

	resp, err := s.killInternal(context.TODO(), &task.KillRequest{
		ID:     t.Name() + "-2",
		ExecID: t.Name() + "-2",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned KillResponse")
	}
}

// TODO: Test_PodShim_execInternal_*

func Test_PodShim_resizePtyInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.resizePtyInternal(context.TODO(), &task.ResizePtyRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_resizePtyInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

	resp, err := s.resizePtyInternal(context.TODO(), &task.ResizePtyRequest{
		ID:     t.Name(),
		ExecID: "thisshouldnotmatch",
	})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_resizePtyInternal_InitTaskID_InitExecID_Success(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

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

func Test_PodShim_resizePtyInternal_2ndTaskID_2ndExecID_Success(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

	resp, err := s.resizePtyInternal(context.TODO(), &task.ResizePtyRequest{
		ID:     t.Name() + "-2",
		ExecID: t.Name() + "-2",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned ResizePtyResponse")
	}
}

func Test_PodShim_closeIOInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.closeIOInternal(context.TODO(), &task.CloseIORequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_closeIOInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

	resp, err := s.closeIOInternal(context.TODO(), &task.CloseIORequest{
		ID:     t.Name(),
		ExecID: "thisshouldnotmatch",
	})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_closeIOInternal_InitTaskID_InitExecID_Success(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

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

func Test_PodShim_closeIOInternal_2ndTaskID_2ndExecID_Success(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

	resp, err := s.closeIOInternal(context.TODO(), &task.CloseIORequest{
		ID:     t.Name() + "-2",
		ExecID: t.Name() + "-2",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned CloseIOResponse")
	}
}

func Test_PodShim_updateInternal_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.updateInternal(context.TODO(), &task.UpdateTaskRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
}

func Test_PodShim_waitInternal_NoTask_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.waitInternal(context.TODO(), &task.WaitRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_waitInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
	s, _, _ := setupPodServiceWithFakes(t)

	resp, err := s.waitInternal(context.TODO(), &task.WaitRequest{
		ID:     t.Name(),
		ExecID: "thisshouldnotmatch",
	})

	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
}

func Test_PodShim_waitInternal_InitTaskID_InitExecID_Success(t *testing.T) {
	s, t1, _ := setupPodServiceWithFakes(t)

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

func Test_PodShim_waitInternal_2ndTaskID_2ndExecID_Success(t *testing.T) {
	s, _, t2 := setupPodServiceWithFakes(t)

	resp, err := s.waitInternal(context.TODO(), &task.WaitRequest{
		ID:     t.Name() + "-2",
		ExecID: t.Name() + "-2",
	})
	if err != nil {
		t.Fatalf("should not have failed with error got: %v", err)
	}
	if resp == nil {
		t.Fatal("should of returned WaitResponse")
	}
	if resp.ExitStatus != t2.execs[t2.id].Status().ExitStatus {
		t.Fatal("should of returned exit status for init")
	}
}

func Test_PodShim_statsInternal_Error(t *testing.T) {
	s := service{
		tid:       t.Name(),
		isSandbox: true,
	}

	resp, err := s.statsInternal(context.TODO(), &task.StatsRequest{ID: t.Name()})

	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
}
