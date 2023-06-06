//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/osversion"
	"github.com/containerd/containerd"
	eventtypes "github.com/containerd/containerd/api/events"
	eventsapi "github.com/containerd/containerd/api/services/events/v1"
	kubeutil "github.com/containerd/containerd/integration/remote/util"
	eventruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/typeurl"
	"github.com/gogo/protobuf/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	testflag "github.com/Microsoft/hcsshim/test/pkg/flag"
	"github.com/Microsoft/hcsshim/test/pkg/images"
	"github.com/Microsoft/hcsshim/test/pkg/require"

	_ "github.com/Microsoft/hcsshim/test/pkg/manifest"
)

const (
	connectTimeout = time.Second * 10
	testNamespace  = "cri-containerd-test"

	lcowRuntimeHandler                = "runhcs-lcow"
	wcowProcessRuntimeHandler         = "runhcs-wcow-process"
	wcowHypervisorRuntimeHandler      = "runhcs-wcow-hypervisor"
	wcowHypervisor17763RuntimeHandler = "runhcs-wcow-hypervisor-17763"
	wcowHypervisor18362RuntimeHandler = "runhcs-wcow-hypervisor-18362"
	wcowHypervisor19041RuntimeHandler = "runhcs-wcow-hypervisor-19041"

	testDeviceUtilFilePath    = "C:\\ContainerPlat\\device-util.exe"
	testJobObjectUtilFilePath = "C:\\ContainerPlat\\jobobject-util.exe"

	testDriversPath  = "C:\\ContainerPlat\\testdrivers"
	testGPUBootFiles = "C:\\ContainerPlat\\LinuxBootFiles\\nvidiagpu"

	testVMServiceAddress = "C:\\ContainerPlat\\vmservice.sock"
	testVMServiceBinary  = "C:\\Containerplat\\vmservice.exe"

	imageLcowK8sPause       = "mcr.microsoft.com/oss/kubernetes/pause:3.1"
	imageLcowAlpine         = "mcr.microsoft.com/mirror/docker/library/alpine:3.16"
	imageLcowAlpineCoreDump = "cplatpublic.azurecr.io/stackoverflow-alpine:latest"
	imageLcowCosmos         = "cosmosarno/spark-master:2.4.1_2019-04-18_8e864ce"
	imageLcowCustomUser     = "cplatpublic.azurecr.io/linux_custom_user:latest"
	imageLcowUbuntu         = "ubuntu:latest"
	alpineAspNet            = "mcr.microsoft.com/dotnet/core/aspnet:3.1-alpine3.11"
	alpineAspnetUpgrade     = "mcr.microsoft.com/dotnet/core/aspnet:3.1.2-alpine3.11"

	imageWindowsProcessDump = "cplatpublic.azurecr.io/crashdump:latest"
	imageWindowsArgsEscaped = "cplatpublic.azurecr.io/argsescaped:latest"
	imageWindowsTimezone    = "cplatpublic.azurecr.io/timezone:latest"

	imageJobContainerHNS     = "cplatpublic.azurecr.io/jobcontainer_hns:latest"
	imageJobContainerETW     = "cplatpublic.azurecr.io/jobcontainer_etw:latest"
	imageJobContainerVHD     = "cplatpublic.azurecr.io/jobcontainer_vhd:latest"
	imageJobContainerCmdline = "cplatpublic.azurecr.io/jobcontainer_cmdline:latest"
	imageJobContainerWorkDir = "cplatpublic.azurecr.io/jobcontainer_workdir:latest"

	gracefulTerminationServercore = "cplatpublic.azurecr.io/servercore-gracefultermination-repro:latest"
	gracefulTerminationNanoserver = "cplatpublic.azurecr.io/nanoserver-gracefultermination-repro:latest"

	// Default account name for use with GMSA related tests. This will not be
	// present/you will not have access to the account on your machine unless
	// your environment is configured properly.
	gmsaAccount = "cplat"
)

// Image definitions
//
//nolint:unused // may be used in future tests
var (
	imageWindowsNanoserver      = getWindowsNanoserverImage(osversion.Build())
	imageWindowsServercore      = getWindowsServerCoreImage(osversion.Build())
	imageWindowsNanoserver17763 = images.ImageWindowsNanoserver1809
	imageWindowsNanoserver18362 = images.ImageWindowsNanoserver1903
	imageWindowsNanoserver19041 = images.ImageWindowsNanoserver2004
	imageWindowsServercore17763 = images.ImageWindowsServercore1809
	imageWindowsServercore18362 = images.ImageWindowsServercore1903
	imageWindowsServercore19041 = images.ImageWindowsServercore2004
)

// Flags
var (
	flagFeatures              = testflag.NewFeatureFlag(allFeatures)
	flagCRIEndpoint           = flag.String("cri-endpoint", "tcp://127.0.0.1:2376", "Address of CRI runtime and image service.")
	flagVirtstack             = flag.String("virtstack", "", "Virtstack to use for hypervisor isolated containers")
	flagVMServiceBinary       = flag.String("vmservice-binary", "", "Path to a binary implementing the vmservice ttrpc service")
	flagContainerdServiceName = flag.String("containerd-service-name", "containerd", "Name of the containerd Windows service")
	flagSevSnp                = flag.Bool("sev-snp", false, "Indicates that the tests are running on hardware with SEV-SNP support")
)

// Features
// Make sure you update allFeatures below with any new features you add.
const (
	featureLCOW               = "LCOW"
	featureWCOWProcess        = "WCOWProcess"
	featureWCOWHypervisor     = "WCOWHypervisor"
	featureHostProcess        = "HostProcess"
	featureGMSA               = "GMSA"
	featureGPU                = "GPU"
	featureCRIUpdateContainer = "UpdateContainer"
	featureTerminateOnRestart = "TerminateOnRestart"
	featureLCOWIntegrity      = "LCOWIntegrity"
	featureLCOWCrypt          = "LCOWCrypt"
	featureCRIPlugin          = "CRIPlugin"
)

var allFeatures = []string{
	featureLCOW,
	featureWCOWProcess,
	featureWCOWHypervisor,
	featureHostProcess,
	featureGMSA,
	featureGPU,
	featureCRIUpdateContainer,
	featureTerminateOnRestart,
	featureLCOWIntegrity,
	featureLCOWCrypt,
	featureCRIPlugin,
}

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

// requireFeatures checks in flagFeatures to validate that each required feature
// was enabled, and skips the test if any are missing. If the flagFeatures set
// is empty, the function returns (by default all features are enabled).
func requireFeatures(tb testing.TB, features ...string) {
	tb.Helper()
	require.Features(tb, flagFeatures, features...)
}

// requireBinary checks if `binary` exists in the same directory as the test
// binary.
// Returns full binary path if it exists, otherwise, skips the test.
func requireBinary(tb testing.TB, binary string) string {
	tb.Helper()
	executable, err := os.Executable()
	if err != nil {
		tb.Skipf("error locating executable: %s", err)
		return ""
	}
	baseDir := filepath.Dir(executable)
	binaryPath := filepath.Join(baseDir, binary)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		tb.Skipf("binary not found: %s", binaryPath)
		return ""
	}
	return binaryPath
}

func getWindowsNanoserverImage(build uint16) string {
	tag, err := images.ImageFromBuild(build)
	if err != nil {
		panic(err)
	}
	return images.NanoserverImage(tag)
}

//nolint:unused // may be used in future tests
func getWindowsServerCoreImage(build uint16) string {
	tag, err := images.ImageFromBuild(build)
	if err != nil {
		panic(err)
	}
	return images.ServercoreImage(tag)
}

func createGRPCConn(ctx context.Context) (*grpc.ClientConn, error) {
	addr, dialer, err := kubeutil.GetAddressAndDialer(*flagCRIEndpoint)
	if err != nil {
		return nil, err
	}
	return grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(dialer))
}

func newTestRuntimeClient(tb testing.TB) runtime.RuntimeServiceClient {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	conn, err := createGRPCConn(ctx)
	if err != nil {
		tb.Fatalf("failed to dial runtime client: %v", err)
	}
	return runtime.NewRuntimeServiceClient(conn)
}

func newTestEventService(tb testing.TB) containerd.EventService {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	conn, err := createGRPCConn(ctx)
	if err != nil {
		tb.Fatalf("Failed to create a client connection %v", err)
	}
	return containerd.NewEventServiceFromClient(eventsapi.NewEventsClient(conn))
}

func newTestImageClient(tb testing.TB) runtime.ImageServiceClient {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	conn, err := createGRPCConn(ctx)
	if err != nil {
		tb.Fatalf("failed to dial runtime client: %v", err)
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

func pullRequiredImages(tb testing.TB, images []string, opts ...SandboxConfigOpt) {
	tb.Helper()
	opts = append(opts, WithSandboxLabels(map[string]string{
		"sandbox-platform": "windows/amd64", // Not required for Windows but makes the test safer depending on defaults in the config.
	}))
	pullRequiredImagesWithOptions(tb, images, opts...)
}

func pullRequiredLCOWImages(tb testing.TB, images []string, opts ...SandboxConfigOpt) {
	tb.Helper()
	opts = append(opts, WithSandboxLabels(map[string]string{
		"sandbox-platform": "linux/amd64",
	}))
	pullRequiredImagesWithOptions(tb, images, opts...)
}

func pullRequiredImagesWithOptions(tb testing.TB, images []string, opts ...SandboxConfigOpt) {
	tb.Helper()
	if len(images) < 1 {
		return
	}

	client := newTestImageClient(tb)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sb := &runtime.PodSandboxConfig{}
	for _, o := range opts {
		if err := o(sb); err != nil {
			tb.Fatalf("failed to apply PodSandboxConfig option: %s", err)
		}
	}

	for _, image := range images {
		_, err := client.PullImage(ctx, &runtime.PullImageRequest{
			Image: &runtime.ImageSpec{
				Image: image,
			},
			SandboxConfig: sb,
		})
		if err != nil {
			tb.Fatalf("failed PullImage for image: %s, with error: %v", image, err)
		}
	}
}

func removeImages(tb testing.TB, images []string) {
	tb.Helper()
	if len(images) < 1 {
		return
	}

	client := newTestImageClient(tb)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, image := range images {
		_, err := client.RemoveImage(ctx, &runtime.RemoveImageRequest{
			Image: &runtime.ImageSpec{
				Image: image,
			},
		})
		if err != nil {
			tb.Fatalf("failed removeImage for image: %s, with error: %v", image, err)
		}
	}
}
