//go:build windows && functional

// This package tests the internals of hcsshim, independent of the OCI interfaces it exposes
// and the container runtime (or CRI API) that normally would be communicating with the shim.
//
// While these tests may overlap with CRI/containerd or shim tests, they exercise `internal/*`
// code paths and primitives directly.
package functional

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/containerd/containerd/namespaces"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/winapi"

	"github.com/Microsoft/hcsshim/test/internal/constants"
	testflag "github.com/Microsoft/hcsshim/test/internal/flag"
	"github.com/Microsoft/hcsshim/test/internal/layers"
	"github.com/Microsoft/hcsshim/test/internal/require"
	"github.com/Microsoft/hcsshim/test/internal/util"
	testuvm "github.com/Microsoft/hcsshim/test/internal/uvm"
)

// owner field for uVMs.
const hcsOwner = "hcsshim-functional-tests"

var (
	alpineImagePaths = &layers.LazyImageLayers{
		Image:    constants.ImageLinuxAlpineLatest,
		Platform: constants.PlatformLinux,
	}
	//TODO: pick appropriate image based on OS build
	nanoserverImagePaths = &layers.LazyImageLayers{
		Image:    constants.ImageWindowsNanoserverLTSC2022,
		Platform: constants.PlatformWindows,
	}
	// wcow tests originally used busyboxw; cannot find image on docker or mcr
	servercoreImagePaths = &layers.LazyImageLayers{
		Image:    constants.ImageWindowsServercoreLTSC2022,
		Platform: constants.PlatformWindows,
	}
)

const (
	featureLCOW        = "LCOW"
	featureWCOW        = "WCOW"
	featureContainer   = "container"
	featureHostProcess = "HostProcess"
	featureUVMMem      = "UVMMem"
	featurePlan9       = "Plan9"
	featureSCSI        = "SCSI"
	featureScratch     = "Scratch"
	featureVSMB        = "vSMB"
	featureVPMEM       = "vPMEM"
)

var allFeatures = []string{
	featureLCOW,
	featureWCOW,
	featureHostProcess,
	featureContainer,
	featureUVMMem,
	featurePlan9,
	featureSCSI,
	featureScratch,
	featureVSMB,
	featureVPMEM,
}

var (
	flagPauseAfterCreateContainerFailure time.Duration

	flagFeatures = testflag.NewFeatureFlag(allFeatures)
	flagDebug    = flag.Bool("debug",
		os.Getenv("HCSSHIM_FUNCTIONAL_TESTS_DEBUG") != "",
		"set logging level to debug [%HCSSHIM_FUNCTIONAL_TESTS_DEBUG%]")
	flagContainerdNamespace = flag.String("ctr-namespace", hcsOwner,
		"containerd `namespace` to use when creating OCI specs")
	flagLCOWLayerPaths = testflag.NewStringSlice("lcow-layer-paths",
		"comma separated list of image layer `paths` to use as LCOW container rootfs. "+
			"If empty, \""+alpineImagePaths.Image+"\" will be pulled and unpacked.")
	//nolint:unused // will be used when WCOW tests are updated
	flagWCOWLayerPaths = testflag.NewStringSlice("wcow-layer-paths",
		"comma separated list of image layer `paths` to use as WCOW uVM and container rootfs. "+
			"If empty, \""+nanoserverImagePaths.Image+"\" will be pulled and unpacked.")
	flagLayerTempDir = flag.String("layer-temp-dir", "",
		"`directory` to unpack image layers to, if not provided. Leave empty to use os.TempDir.")
	flagLinuxBootFilesPath = flag.String("linux-bootfiles", "",
		"override default `path` for LCOW uVM boot files (rootfs.vhd, initrd.img, kernel, and vmlinux)")
)

func init() {
	if !winapi.IsElevated() {
		log.Fatal("tests must be run in an elevated context")
	}

	// This allows for debugging a utility VM.
	if s := os.Getenv("HCSSHIM_FUNCTIONAL_TESTS_PAUSE_ON_CREATECONTAINER_FAIL_IN_MINUTES"); s != "" {
		if t, err := strconv.Atoi(s); err == nil {
			flagPauseAfterCreateContainerFailure = time.Duration(t) * time.Minute
		}
	}
	flag.DurationVar(&flagPauseAfterCreateContainerFailure,
		"container-creation-failure-pause",
		flagPauseAfterCreateContainerFailure,
		"the number of minutes to wait after a container creation failure to try again "+
			"[%HCSSHIM_FUNCTIONAL_TESTS_PAUSE_ON_CREATECONTAINER_FAIL_IN_MINUTES%]")
}

func TestMain(m *testing.M) {
	flag.Parse()

	lvl := logrus.WarnLevel
	if *flagDebug {
		lvl = logrus.DebugLevel
	}
	logrus.SetLevel(lvl)
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	logrus.Infof("using features %q", flagFeatures.S.Strings())

	images := []*layers.LazyImageLayers{alpineImagePaths, nanoserverImagePaths, servercoreImagePaths}
	for _, l := range images {
		l.TempPath = *flagLayerTempDir
	}

	e := m.Run()

	// close any uVMs that escaped
	cmdStr := ` foreach ($vm in Get-ComputeProcess -Owner '` + hcsOwner +
		`') { Write-Output "uVM $($vm.Id) was left running" ; Stop-ComputeProcess -Force -Id $vm.Id } `
	cmd := exec.Command("powershell.exe", "-NoLogo", " -NonInteractive", "-Command", cmdStr)
	o, err := cmd.CombinedOutput()
	s := string(o)
	if err != nil {
		logrus.Warningf("failed to cleanup remaining uVMs with command %q: %s: %v", cmdStr, s, err)
	} else if len(o) > 0 {
		logrus.Warningf("cleaned up left over uVMs: %s", strings.Split(s, "\r\n"))
	}

	// delete downloaded layers; cant use defer, since os.exit does not run them
	for _, l := range images {
		// just ignore errors: they are logged, and no other cleanup possible
		_ = l.Close(context.Background())
	}

	os.Exit(e)
}

func CreateContainerTestWrapper(ctx context.Context, options *hcsoci.CreateOptions) (cow.Container, *resources.Resources, error) {
	if flagPauseAfterCreateContainerFailure != 0 {
		options.DoNotReleaseResourcesOnFailure = true
	}
	s, r, err := hcsoci.CreateContainer(ctx, options)
	if err != nil {
		logrus.Warnf("Test is pausing for %s for debugging CreateContainer failure", flagPauseAfterCreateContainerFailure)
		time.Sleep(flagPauseAfterCreateContainerFailure)
		_ = resources.ReleaseResources(ctx, r, options.HostingSystem, true)
	}

	return s, r, err
}

func requireFeatures(tb testing.TB, features ...string) {
	tb.Helper()
	require.Features(tb, flagFeatures.S, features...)
}

func defaultLCOWOptions(tb testing.TB) *uvm.OptionsLCOW {
	tb.Helper()
	opts := testuvm.DefaultLCOWOptions(tb, util.CleanName(tb.Name()), hcsOwner)
	if p := *flagLinuxBootFilesPath; p != "" {
		opts.BootFilesPath = p
	}
	return opts
}

//nolint:deadcode,unused // will be used when WCOW tests are updated
func defaultWCOWOptions(tb testing.TB) *uvm.OptionsWCOW {
	tb.Helper()
	return uvm.NewDefaultOptionsWCOW(util.CleanName(tb.Name()), hcsOwner)
}

// linuxImageLayers returns image layer paths appropriate for use as a container rootfs.
// If layer paths were provided on the command line, they are returned.
// Otherwise, it pulls an appropriate image.
func linuxImageLayers(ctx context.Context, tb testing.TB) []string {
	tb.Helper()
	if ss := flagLCOWLayerPaths.S.Strings(); len(ss) > 0 {
		return ss
	}
	return alpineImagePaths.Layers(ctx, tb)
}

// windowsImageLayers returns image layer paths appropriate for use as a uVM or container rootfs.
// If layer paths were provided on the command line, they are returned.
// Otherwise, it pulls an appropriate image.
//
//nolint:deadcode,unused // will be used when WCOW tests are updated
func windowsImageLayers(ctx context.Context, tb testing.TB) []string {
	tb.Helper()
	if ss := flagWCOWLayerPaths.S.Strings(); len(ss) > 0 {
		return ss
	}
	return nanoserverImagePaths.Layers(ctx, tb)
}

// windowsServercoreImageLayers returns image layer paths for Windows servercore.
//
// See [windowsImageLayers] for more.
//
//nolint:unused // will be used when WCOW tests are updated
func windowsServercoreImageLayers(ctx context.Context, tb testing.TB) []string {
	tb.Helper()
	return servercoreImagePaths.Layers(ctx, tb)
}

// namespacedContext returns a [context.Context] with the provided namespace added via
// [github.com/containerd/containerd/namespaces.WithNamespace].
func namespacedContext() context.Context {
	return namespaces.WithNamespace(context.Background(), *flagContainerdNamespace)
}
