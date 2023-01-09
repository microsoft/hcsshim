//go:build functional
// +build functional

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
	_ "github.com/Microsoft/hcsshim/test/functional/manifest"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
	"github.com/containerd/containerd"
	eventtypes "github.com/containerd/containerd/api/events"
	eventsapi "github.com/containerd/containerd/api/services/events/v1"
	kubeutil "github.com/containerd/containerd/integration/remote/util"
	eventruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/typeurl"
	"github.com/gogo/protobuf/types"
	"google.golang.org/grpc"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
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

	lcowRuntimeHandler       = "runhcs-lcow"
	imageLcowK8sPause        = "k8s.gcr.io/pause:3.1"
	imageLcowAlpine          = "docker.io/library/alpine:latest"
	imageLcowAlpineCoreDump  = "cplatpublic.azurecr.io/stackoverflow-alpine:latest"
	imageWindowsProcessDump  = "cplatpublic.azurecr.io/crashdump:latest"
	imageLcowCosmos          = "cosmosarno/spark-master:2.4.1_2019-04-18_8e864ce"
	imageLcowCustomUser      = "cplatpublic.azurecr.io/linux_custom_user:latest"
	imageJobContainerHNS     = "cplatpublic.azurecr.io/jobcontainer_hns:latest"
	imageJobContainerETW     = "cplatpublic.azurecr.io/jobcontainer_etw:latest"
	imageJobContainerVHD     = "cplatpublic.azurecr.io/jobcontainer_vhd:latest"
	imageJobContainerCmdline = "cplatpublic.azurecr.io/jobcontainer_cmdline:latest"
	imageJobContainerWorkDir = "cplatpublic.azurecr.io/jobcontainer_workdir:latest"
	alpineAspNet             = "mcr.microsoft.com/dotnet/core/aspnet:3.1-alpine3.11"
	alpineAspnetUpgrade      = "mcr.microsoft.com/dotnet/core/aspnet:3.1.2-alpine3.11"
	// Default account name for use with GMSA related tests. This will not be
	// present/you will not have access to the account on your machine unless
	// your environment is configured properly.
	gmsaAccount                   = "cplat"
	gracefulTerminationServercore = "cplatpublic.azurecr.io/servercore-gracefultermination-repro:latest"
	gracefulTerminationNanoserver = "cplatpublic.azurecr.io/nanoserver-gracefultermination-repro:latest"
)

// Image definitions
var (
	imageWindowsNanoserver      = getWindowsNanoserverImage(osversion.Build())
	imageWindowsServercore      = getWindowsServerCoreImage(osversion.Build())
	imageWindowsNanoserver17763 = getWindowsNanoserverImage(osversion.RS5)
	imageWindowsNanoserver18362 = getWindowsNanoserverImage(osversion.V19H1)
	imageWindowsNanoserver19041 = getWindowsNanoserverImage(osversion.V20H1)
	imageWindowsServercore17763 = getWindowsServerCoreImage(osversion.RS5)
	imageWindowsServercore18362 = getWindowsServerCoreImage(osversion.V19H1)
	imageWindowsServercore19041 = getWindowsServerCoreImage(osversion.V20H1)
)

// Flags
var (
	flagFeatures        = testutilities.NewStringSetFlag()
	flagCRIEndpoint     = flag.String("cri-endpoint", "tcp://127.0.0.1:2376", "Address of CRI runtime and image service.")
	flagVirtstack       = flag.String("virtstack", "", "Virtstack to use for hypervisor isolated containers")
	flagVMServiceBinary = flag.String("vmservice-binary", "", "Path to a binary implementing the vmservice ttrpc service")
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
)

var allFeatures = []string{
	featureLCOW,
	featureWCOWProcess,
	featureWCOWHypervisor,
	featureHostProcess,
	featureGMSA,
	featureGPU,
	featureCRIUpdateContainer,
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

// requireBinary checks if `binary` exists in the same directory as the test
// binary.
// Returns full binary path if it exists, otherwise, skips the test.
func requireBinary(t *testing.T, binary string) string {
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
	switch build {
	case osversion.RS5:
		return "mcr.microsoft.com/windows/nanoserver:1809"
	case osversion.V19H1:
		return "mcr.microsoft.com/windows/nanoserver:1903"
	case osversion.V19H2:
		return "mcr.microsoft.com/windows/nanoserver:1909"
	case osversion.V20H1:
		return "mcr.microsoft.com/windows/nanoserver:2004"
	case osversion.V20H2:
		return "mcr.microsoft.com/windows/nanoserver:2009"
	case osversion.V21H2Server:
		return "mcr.microsoft.com/windows/nanoserver:ltsc2022"
	default:
		// Due to some efforts in improving down-level compatibility for Windows containers (see
		// https://techcommunity.microsoft.com/t5/containers/windows-server-2022-and-beyond-for-containers/ba-p/2712487)
		// the ltsc2022 image should continue to work on builds ws2022 and onwards. With this in mind,
		// if there's no mapping for the host build, just use the Windows Server 2022 image.
		if build > osversion.V21H2Server {
			return "mcr.microsoft.com/windows/nanoserver:ltsc2022"
		}
		panic("unsupported build")
	}
}

func getWindowsServerCoreImage(build uint16) string {
	switch build {
	case osversion.RS5:
		return "mcr.microsoft.com/windows/servercore:1809"
	case osversion.V19H1:
		return "mcr.microsoft.com/windows/servercore:1903"
	case osversion.V19H2:
		return "mcr.microsoft.com/windows/servercore:1909"
	case osversion.V20H1:
		return "mcr.microsoft.com/windows/servercore:2004"
	case osversion.V20H2:
		return "mcr.microsoft.com/windows/servercore:2009"
	case osversion.V21H2Server:
		return "mcr.microsoft.com/windows/servercore:ltsc2022"
	default:
		// Due to some efforts in improving down-level compatibility for Windows containers (see
		// https://techcommunity.microsoft.com/t5/containers/windows-server-2022-and-beyond-for-containers/ba-p/2712487)
		// the ltsc2022 image should continue to work on builds ws2022 and onwards. With this in mind,
		// if there's no mapping for the host build, just use the Windows Server 2022 image.
		if build > osversion.V21H2Server {
			return "mcr.microsoft.com/windows/servercore:ltsc2022"
		}
		panic("unsupported build")
	}
}

func createGRPCConn(ctx context.Context) (*grpc.ClientConn, error) {
	addr, dialer, err := kubeutil.GetAddressAndDialer(*flagCRIEndpoint)
	if err != nil {
		return nil, err
	}
	return grpc.DialContext(ctx, addr, grpc.WithInsecure(), grpc.WithContextDialer(dialer))
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

func pullRequiredImages(t *testing.T, images []string, opts ...SandboxConfigOpt) {
	opts = append(opts, WithSandboxLabels(map[string]string{
		"sandbox-platform": "windows/amd64", // Not required for Windows but makes the test safer depending on defaults in the config.
	}))
	pullRequiredImagesWithOptions(t, images, opts...)
}

func pullRequiredLCOWImages(t *testing.T, images []string, opts ...SandboxConfigOpt) {
	opts = append(opts, WithSandboxLabels(map[string]string{
		"sandbox-platform": "linux/amd64",
	}))
	pullRequiredImagesWithOptions(t, images, opts...)
}

func pullRequiredImagesWithOptions(t *testing.T, images []string, opts ...SandboxConfigOpt) {
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
