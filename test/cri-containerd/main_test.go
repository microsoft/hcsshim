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
	testutilities "github.com/Microsoft/hcsshim/test/internal"
	"github.com/containerd/containerd"
	eventtypes "github.com/containerd/containerd/api/events"
	eventsapi "github.com/containerd/containerd/api/services/events/v1"
	kubeutil "github.com/containerd/containerd/integration/remote/util"
	eventruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/typeurl"
	"github.com/gogo/protobuf/types"
	"google.golang.org/grpc"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/Microsoft/hcsshim/test/internal/constants"
	_ "github.com/Microsoft/hcsshim/test/internal/manifest"
)

const (
	connectTimeout = time.Second * 10
	testNamespace  = "cri-containerd-test"

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

	lcowRuntimeHandler            = "runhcs-lcow"
	imageLcowK8sPause             = "mcr.microsoft.com/oss/kubernetes/pause:3.1"
	imageLcowAlpine               = "mcr.microsoft.com/mirror/docker/library/alpine:3.16"
	imageLcowAlpineCoreDump       = "cplatpublic.azurecr.io/stackoverflow-alpine:latest"
	imageLcowCosmos               = "cosmosarno/spark-master:2.4.1_2019-04-18_8e864ce"
	imageLcowCustomUser           = "cplatpublic.azurecr.io/linux_custom_user:latest"
	imageWindowsProcessDump       = "cplatpublic.azurecr.io/crashdump:latest"
	imageWindowsArgsEscaped       = "cplatpublic.azurecr.io/argsescaped:latest"
	imageWindowsTimezone          = "cplatpublic.azurecr.io/timezone:latest"
	imageJobContainerHNS          = "cplatpublic.azurecr.io/jobcontainer_hns:latest"
	imageJobContainerETW          = "cplatpublic.azurecr.io/jobcontainer_etw:latest"
	imageJobContainerVHD          = "cplatpublic.azurecr.io/jobcontainer_vhd:latest"
	imageJobContainerCmdline      = "cplatpublic.azurecr.io/jobcontainer_cmdline:latest"
	imageJobContainerWorkDir      = "cplatpublic.azurecr.io/jobcontainer_workdir:latest"
	alpineAspNet                  = "mcr.microsoft.com/dotnet/core/aspnet:3.1-alpine3.11"
	alpineAspnetUpgrade           = "mcr.microsoft.com/dotnet/core/aspnet:3.1.2-alpine3.11"
	gracefulTerminationServercore = "cplatpublic.azurecr.io/servercore-gracefultermination-repro:latest"
	gracefulTerminationNanoserver = "cplatpublic.azurecr.io/nanoserver-gracefultermination-repro:latest"
	// Default account name for use with GMSA related tests. This will not be
	// present/you will not have access to the account on your machine unless
	// your environment is configured properly.
	gmsaAccount = "cplat"
)

// Image definitions
//
//nolint:deadcode,unused,varcheck // may be used in future tests
var (
	imageWindowsNanoserver      = getWindowsNanoserverImage(osversion.Build())
	imageWindowsServercore      = getWindowsServerCoreImage(osversion.Build())
	imageWindowsNanoserver17763 = constants.ImageWindowsNanoserver1809
	imageWindowsNanoserver18362 = constants.ImageWindowsNanoserver1903
	imageWindowsNanoserver19041 = constants.ImageWindowsNanoserver2004
	imageWindowsServercore17763 = constants.ImageWindowsServercore1809
	imageWindowsServercore18362 = constants.ImageWindowsServercore1903
	imageWindowsServercore19041 = constants.ImageWindowsServercore2004
)

// Flags
var (
	flagFeatures              = testutilities.NewStringSetFlag()
	flagCRIEndpoint           = flag.String("cri-endpoint", "tcp://127.0.0.1:2376", "Address of CRI runtime and image service.")
	flagVirtstack             = flag.String("virtstack", "", "Virtstack to use for hypervisor isolated containers")
	flagVMServiceBinary       = flag.String("vmservice-binary", "", "Path to a binary implementing the vmservice ttrpc service")
	flagContainerdServiceName = flag.String("containerd-service-name", "containerd", "Name of the containerd Windows service")
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
	t.Helper()
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

// requireBinary checks if `binary` exists in the same directory as the test
// binary.
// Returns full binary path if it exists, otherwise, skips the test.
func requireBinary(t *testing.T, binary string) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Skipf("error locating executable: %s", err)
		return ""
	}
	baseDir := filepath.Dir(executable)
	binaryPath := filepath.Join(baseDir, binary)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Skipf("binary not found: %s", binaryPath)
		return ""
	}
	return binaryPath
}

func getWindowsNanoserverImage(build uint16) string {
	tag, err := constants.ImageFromBuild(build)
	if err != nil {
		panic(err)
	}
	return constants.NanoserverImage(tag)
}

func getWindowsServerCoreImage(build uint16) string {
	tag, err := constants.ImageFromBuild(build)
	if err != nil {
		panic(err)
	}
	return constants.ServercoreImage(tag)
}

func createGRPCConn(ctx context.Context) (*grpc.ClientConn, error) {
	addr, dialer, err := kubeutil.GetAddressAndDialer(*flagCRIEndpoint)
	if err != nil {
		return nil, err
	}
	//nolint:staticcheck //TODO: SA1019: grpc.WithInsecure is deprecated: use WithTransportCredentials and insecure.NewCredentials()
	return grpc.DialContext(ctx, addr, grpc.WithInsecure(), grpc.WithContextDialer(dialer))
}

func newTestRuntimeClient(t *testing.T) runtime.RuntimeServiceClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	conn, err := createGRPCConn(ctx)
	if err != nil {
		t.Fatalf("failed to dial runtime client: %v", err)
	}
	return runtime.NewRuntimeServiceClient(conn)
}

func newTestEventService(t *testing.T) containerd.EventService {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	conn, err := createGRPCConn(ctx)
	if err != nil {
		t.Fatalf("Failed to create a client connection %v", err)
	}
	return containerd.NewEventServiceFromClient(eventsapi.NewEventsClient(conn))
}

func newTestImageClient(t *testing.T) runtime.ImageServiceClient {
	t.Helper()
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

func pullRequiredImages(t *testing.T, images []string, opts ...SandboxConfigOpt) {
	t.Helper()
	opts = append(opts, WithSandboxLabels(map[string]string{
		"sandbox-platform": "windows/amd64", // Not required for Windows but makes the test safer depending on defaults in the config.
	}))
	pullRequiredImagesWithOptions(t, images, opts...)
}

func pullRequiredLCOWImages(t *testing.T, images []string, opts ...SandboxConfigOpt) {
	t.Helper()
	opts = append(opts, WithSandboxLabels(map[string]string{
		"sandbox-platform": "linux/amd64",
	}))
	pullRequiredImagesWithOptions(t, images, opts...)
}

func pullRequiredImagesWithOptions(t *testing.T, images []string, opts ...SandboxConfigOpt) {
	t.Helper()
	if len(images) < 1 {
		return
	}

	client := newTestImageClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sb := &runtime.PodSandboxConfig{}
	for _, o := range opts {
		if err := o(sb); err != nil {
			t.Fatalf("failed to apply PodSandboxConfig option: %s", err)
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
			t.Fatalf("failed PullImage for image: %s, with error: %v", image, err)
		}
	}
}

func removeImages(t *testing.T, images []string) {
	t.Helper()
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
