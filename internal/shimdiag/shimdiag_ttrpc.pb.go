// Code generated by protoc-gen-go-ttrpc. DO NOT EDIT.
// source: github.com/Microsoft/hcsshim/internal/shimdiag/shimdiag.proto
package shimdiag

import (
	context "context"
	ttrpc "github.com/containerd/ttrpc"
)

type ShimDiagService interface {
	DiagExecInHost(context.Context, *ExecProcessRequest) (*ExecProcessResponse, error)
	DiagStacks(context.Context, *StacksRequest) (*StacksResponse, error)
	DiagTasks(context.Context, *TasksRequest) (*TasksResponse, error)
	DiagShare(context.Context, *ShareRequest) (*ShareResponse, error)
	DiagPid(context.Context, *PidRequest) (*PidResponse, error)
}

func RegisterShimDiagService(srv *ttrpc.Server, svc ShimDiagService) {
	srv.RegisterService("containerd.runhcs.v1.diag.ShimDiag", &ttrpc.ServiceDesc{
		Methods: map[string]ttrpc.Method{
			"DiagExecInHost": func(ctx context.Context, unmarshal func(interface{}) error) (interface{}, error) {
				var req ExecProcessRequest
				if err := unmarshal(&req); err != nil {
					return nil, err
				}
				return svc.DiagExecInHost(ctx, &req)
			},
			"DiagStacks": func(ctx context.Context, unmarshal func(interface{}) error) (interface{}, error) {
				var req StacksRequest
				if err := unmarshal(&req); err != nil {
					return nil, err
				}
				return svc.DiagStacks(ctx, &req)
			},
			"DiagTasks": func(ctx context.Context, unmarshal func(interface{}) error) (interface{}, error) {
				var req TasksRequest
				if err := unmarshal(&req); err != nil {
					return nil, err
				}
				return svc.DiagTasks(ctx, &req)
			},
			"DiagShare": func(ctx context.Context, unmarshal func(interface{}) error) (interface{}, error) {
				var req ShareRequest
				if err := unmarshal(&req); err != nil {
					return nil, err
				}
				return svc.DiagShare(ctx, &req)
			},
			"DiagPid": func(ctx context.Context, unmarshal func(interface{}) error) (interface{}, error) {
				var req PidRequest
				if err := unmarshal(&req); err != nil {
					return nil, err
				}
				return svc.DiagPid(ctx, &req)
			},
		},
	})
}

type shimdiagClient struct {
	client *ttrpc.Client
}

func NewShimDiagClient(client *ttrpc.Client) ShimDiagService {
	return &shimdiagClient{
		client: client,
	}
}

func (c *shimdiagClient) DiagExecInHost(ctx context.Context, req *ExecProcessRequest) (*ExecProcessResponse, error) {
	var resp ExecProcessResponse
	if err := c.client.Call(ctx, "containerd.runhcs.v1.diag.ShimDiag", "DiagExecInHost", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *shimdiagClient) DiagStacks(ctx context.Context, req *StacksRequest) (*StacksResponse, error) {
	var resp StacksResponse
	if err := c.client.Call(ctx, "containerd.runhcs.v1.diag.ShimDiag", "DiagStacks", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *shimdiagClient) DiagTasks(ctx context.Context, req *TasksRequest) (*TasksResponse, error) {
	var resp TasksResponse
	if err := c.client.Call(ctx, "containerd.runhcs.v1.diag.ShimDiag", "DiagTasks", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *shimdiagClient) DiagShare(ctx context.Context, req *ShareRequest) (*ShareResponse, error) {
	var resp ShareResponse
	if err := c.client.Call(ctx, "containerd.runhcs.v1.diag.ShimDiag", "DiagShare", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *shimdiagClient) DiagPid(ctx context.Context, req *PidRequest) (*PidResponse, error) {
	var resp PidResponse
	if err := c.client.Call(ctx, "containerd.runhcs.v1.diag.ShimDiag", "DiagPid", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
