//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"flag"
	"os"
	"testing"
	"time"

	kubeutil "github.com/containerd/containerd/v2/integration/remote/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/Microsoft/hcsshim/osversion"
	testflag "github.com/Microsoft/hcsshim/test/pkg/flag"
	"github.com/Microsoft/hcsshim/test/pkg/images"
	"github.com/Microsoft/hcsshim/test/pkg/require"

	_ "github.com/Microsoft/hcsshim/test/pkg/manifest"
)

const (
	connectTimeout = time.Second * 10
	testNamespace  = "cri-containerd-test"

	// TODO: remove lcow when shim only tests are relocated
	lcowRuntimeHandler                = "runhcs-lcow"
	wcowProcessRuntimeHandler         = "runhcs-wcow-process"
	wcowHypervisorRuntimeHandler      = "runhcs-wcow-hypervisor"
	wcowHypervisor17763RuntimeHandler = "runhcs-wcow-hypervisor-17763"
	wcowHypervisor18362RuntimeHandler = "runhcs-wcow-hypervisor-18362"
	wcowHypervisor19041RuntimeHandler = "runhcs-wcow-hypervisor-19041"

	testDeviceUtilFilePath    = "C:\\ContainerPlat\\device-util.exe"
	testJobObjectUtilFilePath = "C:\\ContainerPlat\\jobobject-util.exe"

	testDriversPath = "C:\\ContainerPlat\\testdrivers"

	testVMServiceAddress = "C:\\ContainerPlat\\vmservice.sock"
	testVMServiceBinary  = "C:\\Containerplat\\vmservice.exe"

	// TODO: remove the lcow ones when shim only tests are relocated.
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

	imageJobContainerHNS     = "cplatpublic.azurecr.io/hpc_hns:latest"
	imageJobContainerETW     = "cplatpublic.azurecr.io/hpc_etw:latest"
	imageJobContainerVHD     = "cplatpublic.azurecr.io/hpc_vhd:latest"
	imageJobContainerCmdline = "cplatpublic.azurecr.io/hpc_cmdline:latest"
	imageJobContainerWorkDir = "cplatpublic.azurecr.io/hpc_workdir:latest"

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
	flagFeatures        = testflag.NewFeatureFlag(allFeatures)
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
	featureTerminateOnRestart = "TerminateOnRestart"
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

// requireAnyFeatures checks in flagFeatures if at least one of the required features
// was enabled, and skips the test if all are missing.
//
// See: [requireFeatures]
func requireAnyFeature(tb testing.TB, features ...string) {
	tb.Helper()
	require.AnyFeature(tb, flagFeatures, features...)
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

	// TODO: use grpc.NewClient here instead
	//nolint:staticcheck // SA1019: grpc.DialContext is deprecated, replace with grpc.NewClient
	return grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dialer))
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
