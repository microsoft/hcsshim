//go:build windows

package main

import (
	"context"
	"time"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// newTestShimExec creates a test exec. If you are intending to make an init
// exec `tid==id` MUST be true by containerd convention.
func newTestShimExec(tid, id string, pid int) *testShimExec {
	return &testShimExec{
		tid:   tid,
		id:    id,
		pid:   pid,
		state: shimExecStateCreated,
	}
}

var _ = (shimExec)(&testShimExec{})

type testShimExec struct {
	tid    string
	id     string
	pid    int
	status uint32
	at     time.Time

	state shimExecState
}

func (tse *testShimExec) ID() string {
	return tse.id
}
func (tse *testShimExec) Pid() int {
	return tse.pid
}
func (tse *testShimExec) State() shimExecState {
	return tse.state
}
func (tse *testShimExec) Status() *task.StateResponse {
	return &task.StateResponse{
		ID:         tse.tid,
		ExecID:     tse.id,
		Pid:        uint32(tse.pid),
		ExitStatus: tse.status,
		ExitedAt:   timestamppb.New(tse.at),
	}
}
func (tse *testShimExec) Start(ctx context.Context) error {
	tse.state = shimExecStateRunning
	tse.status = 255
	return nil
}
func (tse *testShimExec) Kill(ctx context.Context, signal uint32) error {
	tse.state = shimExecStateExited
	tse.status = 0
	tse.at = time.Now()
	return nil
}
func (tse *testShimExec) ResizePty(ctx context.Context, width, height uint32) error {
	return nil
}
func (tse *testShimExec) CloseIO(ctx context.Context, stdin bool) error {
	return nil
}
func (tse *testShimExec) Wait() *task.StateResponse {
	return tse.Status()
}
func (tse *testShimExec) ForceExit(ctx context.Context, status int) {
	if tse.state != shimExecStateExited {
		tse.state = shimExecStateExited
		tse.status = 1
		tse.at = time.Now()
	}
}
