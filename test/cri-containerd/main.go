// +build functional

package cri_containerd

import (
	"context"
	"net"
	"testing"
	"time"

	_ "github.com/Microsoft/hcsshim/test/functional/manifest"
	"google.golang.org/grpc"
	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

const (
	daemonAddress                = "tcp://127.0.0.1:2376"
	connectTimeout               = time.Second * 10
	testNamespace                = "cri-containerd-test"
	wcowProcessRuntimeHandler    = "default-debug"
	wcowHypervisorRuntimeHandler = "wcow-debug"
	lcowRuntimeHandler           = "lcow-debug"
)

func newTestRuntimeClient(t *testing.T) runtime.RuntimeServiceClient {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, daemonAddress, grpc.WithInsecure(), grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("tcp", "127.0.0.1:2376", timeout)
	}))
	if err != nil {
		t.Fatalf("failed to dial runtime client: %v", err)
	}
	return runtime.NewRuntimeServiceClient(conn)
}
