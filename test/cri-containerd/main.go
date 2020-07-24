// +build functional

package cri_containerd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/osversion"
	_ "github.com/Microsoft/hcsshim/test/functional/manifest"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
	"github.com/containerd/containerd"
	eventtypes "github.com/containerd/containerd/api/events"
	eventsapi "github.com/containerd/containerd/api/services/events/v1"
	eventruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/typeurl"
	"github.com/gogo/protobuf/types"
	"google.golang.org/grpc"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	daemonAddress                     = "tcp://127.0.0.1:2376"
	connectTimeout                    = time.Second * 10
	testNamespace                     = "cri-containerd-test"
	wcowProcessRuntimeHandler         = "runhcs-wcow-process"
	wcowHypervisorRuntimeHandler      = "runhcs-wcow-hypervisor"
	wcowHypervisor17763RuntimeHandler = "runhcs-wcow-hypervisor-17763"
	wcowHypervisor18362RuntimeHandler = "runhcs-wcow-hypervisor-18362"
	wcowHypervisor19041RuntimeHandler = "runhcs-wcow-hypervisor-19041"
	lcowRuntimeHandler                = "runhcs-lcow"
	imageLcowK8sPause                 = "k8s.gcr.io/pause:3.1"
	imageLcowAlpine                   = "docker.io/library/alpine:latest"
	imageLcowCosmos                   = "cosmosarno/spark-master:2.4.1_2019-04-18_8e864ce"
	testGPUBootFiles                  = "C:\\ContainerPlat\\LinuxBootFiles\\nvidiagpu"
	alpineAspNet                      = "mcr.microsoft.com/dotnet/core/aspnet:3.1-alpine3.11"
	alpineAspnetUpgrade               = "mcr.microsoft.com/dotnet/core/aspnet:3.1.2-alpine3.11"
	// Default account name for use with GMSA related tests. This will not be
	// present/you will not have access to the account on your machine unless
	// your environment is configured properly.
	gmsaAccount = "cplat"
)

// Image definitions
var (
	imageWindowsNanoserver      = getWindowsNanoserverImage(osversion.Get().Build)
	imageWindowsServercore      = getWindowsServerCoreImage(osversion.Get().Build)
	imageWindowsNanoserver17763 = getWindowsNanoserverImage(osversion.RS5)
	imageWindowsNanoserver18362 = getWindowsNanoserverImage(osversion.V19H1)
	imageWindowsNanoserver19041 = getWindowsNanoserverImage(osversion.V20H1)
	imageWindowsServercore17763 = getWindowsServerCoreImage(osversion.RS5)
	imageWindowsServercore18362 = getWindowsServerCoreImage(osversion.V19H1)
	imageWindowsServercore19041 = getWindowsServerCoreImage(osversion.V20H1)
)

// Flags
var (
	flagFeatures = testutilities.NewStringSetFlag()
)

// Features
// Make sure you update allFeatures below with any new features you add.
const (
	featureLCOW           = "LCOW"
	featureWCOWProcess    = "WCOWProcess"
	featureWCOWHypervisor = "WCOWHypervisor"
	featureGMSA           = "GMSA"
	featureGPU            = "GPU"
)

var allFeatures = []string{
	featureLCOW,
	featureWCOWProcess,
	featureWCOWHypervisor,
	featureGMSA,
	featureGPU,
}

func init() {
	// Flag definitions must be in init rather than TestMain, as TestMain isn't
	// called if -help is passed, but we want the feature usage to show up.
	flag.Var(flagFeatures, "feature", fmt.Sprintf(
		"specifies which sets of functionality to test. can be set multiple times\n"+
			"supported features: %v", allFeatures))
}

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

// requireFeatures checks in flagFeatures to validate that each required feature
// was enabled, and skips the test if any are missing. If the flagFeatures set
// is empty, the function returns (by default all features are enabled).
func requireFeatures(t *testing.T, features ...string) {
	set := flagFeatures.ValueSet()
	if len(set) == 0 {
		return
	}
	for _, feature := range features {
		if _, ok := set[feature]; !ok {
			t.Skipf("skipping test due to feature not set: %s", feature)
		}
	}
}

func getWindowsNanoserverImage(build uint16) string {
	switch build {
	case osversion.RS5:
		return "mcr.microsoft.com/windows/nanoserver:1809"
	case osversion.V19H1:
		return "mcr.microsoft.com/windows/nanoserver:1903"
	case osversion.V20H1:
		return "mcr.microsoft.com/windows/nanoserver:2004"
	default:
		panic("unsupported build")
	}
}

func getWindowsServerCoreImage(build uint16) string {
	switch build {
	case osversion.RS5:
		return "mcr.microsoft.com/windows/servercore:1809"
	case osversion.V19H1:
		return "mcr.microsoft.com/windows/servercore:1903"
	case osversion.V20H1:
		return "mcr.microsoft.com/windows/servercore:2004"
	default:
		panic("unsupported build")
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

func removeImages(t *testing.T, images []string) {
	if len(images) < 1 {
		return
	}

	client := newTestImageClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, image := range images {
		_, err := client.RemoveImage(ctx, &runtime.RemoveImageRequest{
			Image: &runtime.ImageSpec{
				Image: image,
			},
		})
		if err != nil {
			t.Fatalf("failed removeImage for image: %s, with error: %v", image, err)
		}
	}
}
