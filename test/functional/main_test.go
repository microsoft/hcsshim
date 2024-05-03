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
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio/pkg/etw"
	"github.com/Microsoft/go-winio/pkg/etwlogrus"
	"github.com/containerd/containerd/namespaces"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/Microsoft/hcsshim/internal/sync"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/Microsoft/hcsshim/osversion"

	testlayers "github.com/Microsoft/hcsshim/test/internal/layers"
	"github.com/Microsoft/hcsshim/test/internal/util"
	testflag "github.com/Microsoft/hcsshim/test/pkg/flag"
	testimages "github.com/Microsoft/hcsshim/test/pkg/images"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

// owner field for uVMs.
const hcsOwner = "hcsshim-functional-tests"

// how long to allow a benchmark iteration to run for.
const benchmarkIterationTimeout = 30 * time.Second

// Linux image(s)

var alpineImagePaths = &testlayers.LazyImageLayers{
	Image:        testimages.ImageLinuxAlpineLatest,
	Platform:     testimages.PlatformLinux,
	AppendVerity: false, // will be set to true if [featureLCOWIntegrity] is passed
}

// Windows images

// group wcow images together for shared init.
// want to avoid erroring if WCOW tests are not selected, and also prevet accidentally accessing values without checking
// error value first.
type wcowImages struct {
	nanoserver *testlayers.LazyImageLayers

	// wcow tests originally used busyboxw; cannot find image on docker or mcr.
	servercore *testlayers.LazyImageLayers
}

var wcowImagePathsOnce = sync.OnceValue(func() (*wcowImages, error) {
	build := osversion.Build()
	tag, err := testimages.ImageFromBuild(build)
	if err != nil || tag == "" {
		return nil, fmt.Errorf("Windows images init: could not look up image tag for build %d", build)
	}

	return &wcowImages{
		nanoserver: &testlayers.LazyImageLayers{
			Image:    testimages.NanoserverImage(tag),
			Platform: testimages.PlatformWindows,
		},
		servercore: &testlayers.LazyImageLayers{
			Image:    testimages.ServercoreImage(tag),
			Platform: testimages.PlatformWindows,
		},
	}, nil
})

const (
	// container and uVM types.

	featureLCOW          = "LCOW"          // Linux containers or uVM tests; requires [featureUVM]
	featureLCOWIntegrity = "LCOWIntegrity" // Linux confidential/policy tests
	featureWCOW          = "WCOW"          // Windows containers or uVM tests
	featureUVM           = "uVM"           // tests that create a utility VM
	featureContainer     = "container"     // tests that create a container (either process or hyper-v isolated)
	featureHostProcess   = "HostProcess"   // tests that create a Windows HostProcess container; requires [featureWCOW]

	// resources and misc functionality.

	featureScratch = "Scratch" // validate scratch layer mounting
	featurePlan9   = "Plan9"   // Plan9 file shares
	featureSCSI    = "SCSI"    // SCSI disk (virtuall and physical) mounts
	featureVSMB    = "vSMB"    // virtual SMB file shares
	featureVPMEM   = "vPMEM"   // virtual PMEM mounts
)

var allFeatures = []string{
	featureLCOW,
	featureLCOWIntegrity,
	featureWCOW,
	featureUVM,
	featureContainer,
	featureHostProcess,
	featureScratch,
	featurePlan9,
	featureSCSI,
	featureVSMB,
	featureVPMEM,
}

var (
	flagLogLevel            = testflag.NewLogrusLevel("log-level", logrus.WarnLevel.String(), "logrus logging `level`")
	flagFeatures            = testflag.NewFeatureFlag(allFeatures)
	flagContainerdNamespace = flag.String("ctr-namespace", hcsOwner,
		"containerd `namespace` to use when creating OCI specs")
	flagLCOWLayerPaths = testflag.NewStringSet("lcow-layer-paths",
		"comma separated list of image layer `paths` to use as LCOW container rootfs. "+
			"If empty, \""+alpineImagePaths.Image+"\" will be pulled and unpacked.", true)
	flagWCOWLayerPaths = testflag.NewStringSet("wcow-layer-paths",
		"comma separated list of image layer `paths` to use as WCOW uVM and container rootfs. "+
			"If empty, \""+testimages.NanoserverImage("")+"\" will be pulled and unpacked.", true)
	flagLayerTempDir = flag.String("layer-temp-dir", "",
		"`directory` to unpack image layers to, if not provided. Leave empty to use os.TempDir.")
	flagLinuxBootFilesPath = flag.String("linux-bootfiles", "",
		"override default `path` for LCOW uVM boot files (rootfs.vhd, initrd.img, kernel, and vmlinux)")
)

func TestMain(m *testing.M) {
	flag.Parse()

	if err := runTests(m); err != nil {
		fmt.Fprintln(os.Stderr, err)

		// if `m.Run()` returns an exit code, use that
		// otherwise, use exit code `1`
		c := 1
		if ec, ok := err.(cli.ExitCoder); ok { //nolint:errorlint
			c = ec.ExitCode()
		}
		os.Exit(c)
	}
}

func runTests(m *testing.M) error {
	// ! don't call os.Exit/log.Fatal here, sine that will skip deferred statements

	ctx := context.Background()

	if !winapi.IsElevated() {
		return fmt.Errorf("tests must be run in an elevated context")
	}

	otel.SetTracerProvider(sdktrace.NewTracerProvider(
		sdktrace.WithSampler(otelutil.DefaultSampler),
		sdktrace.WithBatcher(&otelutil.LogrusExporter{}),
	))

	// default is stderr, but test2json does not consume stderr, so logs would be out of sync
	// and powershell considers output on stderr as an error when execing
	//
	// ! keep defer statement in [util.RunningBenchmarks()] in sync with output/formatter settings here
	logrus.SetOutput(os.Stdout)
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	logrus.SetLevel(flagLogLevel.Level)

	logrus.Debugf("using features: %s", flagFeatures.Strings())

	if flagFeatures.IsSet(featureLCOWIntegrity) {
		logrus.Info("appending verity information to LCOW images")
		alpineImagePaths.AppendVerity = true
	}

	imgs := []*testlayers.LazyImageLayers{}
	if flagFeatures.IsSet(featureLCOWIntegrity) || flagFeatures.IsSet(featureLCOW) {
		imgs = append(imgs, alpineImagePaths)
	}

	if flagFeatures.IsSet(featureWCOW) {
		wcow, err := wcowImagePathsOnce()
		if err != nil {
			return err
		}

		logrus.WithField("image", wcow.nanoserver.Image).Info("using Nano Server image")
		logrus.WithField("image", wcow.servercore.Image).Info("using Server Core image")

		imgs = append(imgs, wcow.nanoserver, wcow.servercore)
	}

	for _, l := range imgs {
		l.TempPath = *flagLayerTempDir
	}

	defer func(ctx context.Context) {
		cleanupComputeSystems(ctx, hcsOwner)

		for _, l := range imgs {
			if l == nil {
				continue
			}
			// just log errors: no other cleanup possible
			if err := l.Close(ctx); err != nil {
				log.G(ctx).WithFields(logrus.Fields{
					logrus.ErrorKey: err,
					"image":         l.Image,
					"platform":      l.Platform,
				}).Warning("image cleanup failed")
			}
		}
	}(ctx)

	// print additional configuration options when running benchmarks, so we can track performance.
	//
	// also, print to ETW instead of stdout to mirror actual deployments, and to prevent logs from
	// interfering with benchmarking output
	if util.RunningBenchmarks() {
		util.PrintAdditionalBenchmarkConfig()

		provider, err := etw.NewProviderWithOptions("Microsoft.Virtualization.RunHCS")
		if err != nil {
			logrus.Error(err)
		} else {
			if hook, err := etwlogrus.NewHookFromProvider(provider); err == nil {
				logrus.AddHook(hook)
			} else {
				logrus.WithError(err).Error("could not create ETW logrus hook")
			}
		}

		// regardless of ETW provider status, still discard logs
		logrus.SetFormatter(log.NopFormatter{})
		logrus.SetOutput(io.Discard)

		defer func() {
			// un-discard logs during cleanup
			logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
			logrus.SetOutput(os.Stdout)
		}()
	}

	if e := m.Run(); e != 0 {
		return cli.Exit("", e)
	}
	return nil
}

// misc helper functions/"global" values

func requireFeatures(tb testing.TB, features ...string) {
	tb.Helper()
	require.Features(tb, flagFeatures, features...)
}

func defaultLCOWOptions(ctx context.Context, tb testing.TB) *uvm.OptionsLCOW {
	tb.Helper()

	opts := testuvm.DefaultLCOWOptions(ctx, tb, testName(tb), hcsOwner)
	if p := *flagLinuxBootFilesPath; p != "" {
		opts.UpdateBootFilesPath(ctx, p)
	}
	return opts
}

func defaultWCOWOptions(ctx context.Context, tb testing.TB) *uvm.OptionsWCOW {
	tb.Helper()

	opts := testuvm.DefaultWCOWOptions(ctx, tb, testName(tb), hcsOwner)
	uvmLayers := windowsImageLayers(ctx, tb)
	scratchDir := testlayers.WCOWScratchDir(ctx, tb, "")
	bootFiles, err := layers.GetWCOWUVMBootFilesFromLayers(ctx, nil, append(uvmLayers, scratchDir))
	if err != nil {
		tb.Fatalf("failed to parse WCOW Boot files: %s", err)
	}
	opts.BootFiles = bootFiles
	return opts
}

func testName(tb testing.TB, xs ...any) string {
	tb.Helper()

	return util.CleanName(tb.Name()) + util.RandNameSuffix(xs...)
}

// linuxImageLayers returns image layer paths appropriate for use as a container rootfs.
// If layer paths were provided on the command line, they are returned.
// Otherwise, it pulls an appropriate image.
func linuxImageLayers(ctx context.Context, tb testing.TB) []string {
	tb.Helper()
	if ss := flagLCOWLayerPaths.Strings(); len(ss) > 0 {
		return ss
	}
	return alpineImagePaths.Layers(ctx, tb)
}

// windowsImageLayers returns image layer paths appropriate for use as a uVM or container rootfs.
// If layer paths were provided on the command line, they are returned.
// Otherwise, it pulls an appropriate image.
func windowsImageLayers(ctx context.Context, tb testing.TB) []string {
	tb.Helper()
	if ss := flagWCOWLayerPaths.Strings(); len(ss) > 0 {
		return ss
	}

	// should have checked error value before running tests, but just in case...
	wcow, err := wcowImagePathsOnce()
	if err != nil {
		tb.Fatalf("could not get Windows Nano Server image: %v", err)
	}

	return wcow.nanoserver.Layers(ctx, tb)
}

// windowsServercoreImageLayers returns image layer paths for Windows servercore.
//
// See [windowsImageLayers] for more.
func windowsServercoreImageLayers(ctx context.Context, tb testing.TB) []string {
	tb.Helper()

	wcow, err := wcowImagePathsOnce()
	if err != nil {
		tb.Fatalf("could not get Windows Server Core image: %v", err)
	}

	return wcow.servercore.Layers(ctx, tb)
}

// namespacedContext returns a [context.Context] with the provided namespace added via
// [github.com/containerd/containerd/namespaces.WithNamespace].
func namespacedContext(ctx context.Context) context.Context {
	// since this (usually) called at the start of a test, add the testing timeout to it
	// for the entire test run
	return namespaces.WithNamespace(ctx, *flagContainerdNamespace)
}

// cleanupComputeSystems close any uVMs or containers that escaped during tests.
func cleanupComputeSystems(ctx context.Context, owner string) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NoLogo", "-NonInteractive", "-Command",
		`foreach ( $s in Get-ComputeProcess -Owner '`+owner+`' ) { `+
			`Write-Output $s.Id ; $null = Stop-ComputeProcess -Force -Id $s.Id`+
			` }`,
	)

	e := log.G(ctx).WithFields(logrus.Fields{
		"cmd":   cmd.String(),
		"owner": owner,
	})
	e.Debug("removing leftover compute systems")

	o, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(o))
	if err != nil {
		e.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"output":        s,
		}).Warning("failed to cleanup leftover compute systems")
	} else if len(o) > 0 {
		e.WithField(
			"systems", strings.Split(s, "\r\n"), // cmd should output one ID per line
		).Warning("cleaned up leftover compute systems")
	}
}
