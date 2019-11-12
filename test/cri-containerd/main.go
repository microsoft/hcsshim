// +build functional

package cri_containerd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/osversion"
	_ "github.com/Microsoft/hcsshim/test/functional/manifest"
	"github.com/containerd/containerd"
	eventtypes "github.com/containerd/containerd/api/events"
	eventsapi "github.com/containerd/containerd/api/services/events/v1"
	eventruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/typeurl"
	"github.com/gogo/protobuf/types"
	"google.golang.org/grpc"
	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

const (
	daemonAddress                     = "tcp://127.0.0.1:2376"
	connectTimeout                    = time.Second * 10
	testNamespace                     = "cri-containerd-test"
	wcowProcessRuntimeHandler         = "runhcs-wcow-process"
	wcowHypervisorRuntimeHandler      = "runhcs-wcow-hypervisor"
	wcowHypervisor17763RuntimeHandler = "runhcs-wcow-hypervisor-17763"
	wcowHypervisor18362RuntimeHandler = "runhcs-wcow-hypervisor-18362"
	lcowRuntimeHandler                = "runhcs-lcow"
	imageLcowK8sPause                 = "k8s.gcr.io/pause:3.1"
	imageLcowAlpine                   = "docker.io/library/alpine:latest"
	imageLcowCosmos                   = "cosmosarno/spark-master:2.4.1_2019-04-18_8e864ce"
)

var (
	imageWindowsNanoserver      = getWindowsNanoserverImage(osversion.Get().Build)
	imageWindowsServercore      = getWindowsServerCoreImage(osversion.Get().Build)
	imageWindowsNanoserver17763 = getWindowsNanoserverImage(osversion.RS5)
	imageWindowsNanoserver18362 = getWindowsNanoserverImage(osversion.V19H1)
	imageWindowsServercore17763 = getWindowsServerCoreImage(osversion.RS5)
	imageWindowsServercore18362 = getWindowsServerCoreImage(osversion.V19H1)
)

func getWindowsNanoserverImage(build uint16) string {
	switch build {
	case osversion.RS5:
		return "mcr.microsoft.com/windows/nanoserver:1809"
	case osversion.V19H1:
		return "mcr.microsoft.com/windows/nanoserver:1903"
	default:
		panic("unspported build")
	}
}

func getWindowsServerCoreImage(build uint16) string {
	switch build {
	case osversion.RS5:
		return "mcr.microsoft.com/windows/servercore:1809"
	case osversion.V19H1:
		return "mcr.microsoft.com/windows/servercore:1903"
	default:
		panic("unspported build")
	}
}

func createGRPCConn(ctx context.Context) (*grpc.ClientConn, error) {
	conn, err := grpc.DialContext(ctx, daemonAddress, grpc.WithInsecure(), grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("tcp", "127.0.0.1:2376", timeout)
	}))
	return conn, err
}

func newTestRuntimeClient(t *testing.T) runtime.RuntimeServiceClient {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	conn, err := createGRPCConn(ctx)
	if err != nil {
		t.Fatalf("failed to dial runtime client: %v", err)
	}
	return runtime.NewRuntimeServiceClient(conn)
}

func newTestEventService(t *testing.T) containerd.EventService {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	conn, err := createGRPCConn(ctx)
	if err != nil {
		t.Fatalf("Failed to create a client connection %v", err)
	}
	return containerd.NewEventServiceFromClient(eventsapi.NewEventsClient(conn))
}

func newTestImageClient(t *testing.T) runtime.ImageServiceClient {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	conn, err := createGRPCConn(ctx)
	if err != nil {
		t.Fatalf("failed to dial runtime client: %v", err)
	}
	return runtime.NewImageServiceClient(conn)
}

func getTargetRunTopics() (topicNames []string, filters []string) {
	topicNames = []string{
		eventruntime.TaskCreateEventTopic,
		eventruntime.TaskStartEventTopic,
		eventruntime.TaskExitEventTopic,
		eventruntime.TaskDeleteEventTopic,
	}

	filters = make([]string, len(topicNames))

	for i, name := range topicNames {
		filters[i] = fmt.Sprintf(`topic=="%v"`, name)
	}
	return topicNames, filters
}

func convertEvent(e *types.Any) (string, interface{}, error) {
	id := ""
	evt, err := typeurl.UnmarshalAny(e)
	if err != nil {
		return "", nil, err
	}

	switch event := evt.(type) {
	case *eventtypes.TaskCreate:
		id = event.ContainerID
	case *eventtypes.TaskStart:
		id = event.ContainerID
	case *eventtypes.TaskDelete:
		id = event.ContainerID
	case *eventtypes.TaskExit:
		id = event.ContainerID
	default:
		return "", nil, errors.New("test does not support this event")
	}
	return id, evt, nil
}

func pullRequiredImages(t *testing.T, images []string) {
	pullRequiredImagesWithLabels(t, images, map[string]string{
		"sandbox-platform": "windows/amd64", // Not required for Windows but makes the test safer depending on defaults in the config.
	})
}

func pullRequiredLcowImages(t *testing.T, images []string) {
	pullRequiredImagesWithLabels(t, images, map[string]string{
		"sandbox-platform": "linux/amd64",
	})
}

func pullRequiredImagesWithLabels(t *testing.T, images []string, labels map[string]string) {
	if len(images) < 1 {
		return
	}

	client := newTestImageClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sb := &runtime.PodSandboxConfig{
		Labels: labels,
	}
	for _, image := range images {
		_, err := client.PullImage(ctx, &runtime.PullImageRequest{
			Image: &runtime.ImageSpec{
				Image: image,
			},
			SandboxConfig: sb,
		})
		if err != nil {
			t.Fatalf("failed PullImage for image: %s, with error: %v", image, err)
		}
	}
}

// findTestDevices returns the first nvidia pcip device on the host
func findTestNvidiaGPUDevice() (string, error) {
	out, err := exec.Command(
		"powershell",
		`(Get-PnpDevice -presentOnly | where-object {$_.InstanceID -Match 'PCIP\\VEN_10DE.*'})[0].InstanceId`,
	).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
