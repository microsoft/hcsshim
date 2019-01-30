package main

import (
	"context"
	"sync"

	"github.com/containerd/containerd/errdefs"
)

var _ = (shimPod)(&testShimPod{})

type testShimPod struct {
	id string

	tasks sync.Map
}

func (tsp *testShimPod) ID() string {
	return tsp.id
}

func (tsp *testShimPod) GetTask(tid string) (shimTask, error) {
	v, loaded := tsp.tasks.Load(tid)
	if loaded {
		return v.(shimTask), nil
	}
	return nil, errdefs.ErrNotFound
}

func (tsp *testShimPod) KillTask(ctx context.Context, tid, eid string, signal uint32, all bool) error {
	s, err := tsp.GetTask(tid)
	if err != nil {
		return err
	}
	return s.KillExec(ctx, eid, signal, all)
}
