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
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/containerd/containerd"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/winapi"

	testctrd "github.com/Microsoft/hcsshim/test/internal/containerd"
	testflag "github.com/Microsoft/hcsshim/test/internal/flag"
	"github.com/Microsoft/hcsshim/test/internal/require"
	testuvm "github.com/Microsoft/hcsshim/test/internal/uvm"
)

// owner field for uVMs.
const hcsOwner = "hcsshim-functional-tests"

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

// todo: use a new containerd namespace and then nuke everything in it

var (
	debug                                 bool
	pauseDurationOnCreateContainerFailure time.Duration

	flagFeatures            = testflag.NewFeatureFlag(allFeatures)
	flagContainerdAddress   = flag.String("ctr-address", "tcp://127.0.0.1:2376", "`address` for containerd's GRPC server")
	flagContainerdNamespace = flag.String("ctr-namespace", "k8s.io", "containerd `namespace`")
	flagLinuxBootFilesPath  = flag.String("linux-bootfiles", "",
		"override default `path` for LCOW uVM boot files (rootfs.vhd, initrd.img, kernel, and vmlinux)")
)

func init() {
	if !winapi.IsElevated() {
		log.Fatal("tests must be run in an elevated context")
	}

	if _, ok := os.LookupEnv("HCSSHIM_FUNCTIONAL_TESTS_DEBUG"); ok {
		debug = true
	}
	flag.BoolVar(&debug, "debug", debug, "set logging level to debug [%HCSSHIM_FUNCTIONAL_TESTS_DEBUG%]")

	// This allows for debugging a utility VM.
	if s := os.Getenv("HCSSHIM_FUNCTIONAL_TESTS_PAUSE_ON_CREATECONTAINER_FAIL_IN_MINUTES"); s != "" {
		if t, err := strconv.Atoi(s); err == nil {
			pauseDurationOnCreateContainerFailure = time.Duration(t) * time.Minute
		}
	}
	flag.DurationVar(&pauseDurationOnCreateContainerFailure,
		"container-creation-failure-pause",
		pauseDurationOnCreateContainerFailure,
		"the number of minutes to wait after a container creation failure to try again "+
			"[%HCSSHIM_FUNCTIONAL_TESTS_PAUSE_ON_CREATECONTAINER_FAIL_IN_MINUTES%]")
}

func TestMain(m *testing.M) {
	flag.Parse()

	lvl := logrus.WarnLevel
	if debug {
		lvl = logrus.DebugLevel
	}
	logrus.SetLevel(lvl)
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	logrus.Infof("using features %q", flagFeatures.S.Strings())

	e := m.Run()

	// close any uVMs that escaped
	cmdStr := `foreach ($vm in Get-ComputeProcess -Owner '` + hcsOwner + `') ` +
		`{ Write-Output $vm.Id ; Stop-ComputeProcess -Force -Id $vm.Id }`
	cmd := exec.Command("powershell.exe", "-NoLogo", " -NonInteractive", "-Command", cmdStr)
	o, err := cmd.CombinedOutput()
	s := string(o)
	if err != nil {
		logrus.Warningf("failed to cleanup remaining uVMs with command %q: %s: %v", cmdStr, s, err)
	} else if len(o) > 0 {
		logrus.Warningf("cleaned up left over uVMs: %s", strings.Split(s, "\r\n"))
	}

	os.Exit(e)
}

func CreateContainerTestWrapper(ctx context.Context, options *hcsoci.CreateOptions) (cow.Container, *resources.Resources, error) {
	if pauseDurationOnCreateContainerFailure != 0 {
		options.DoNotReleaseResourcesOnFailure = true
	}
	s, r, err := hcsoci.CreateContainer(ctx, options)
	if err != nil {
		logrus.Warnf("Test is pausing for %s for debugging CreateContainer failure", pauseDurationOnCreateContainerFailure)
		time.Sleep(pauseDurationOnCreateContainerFailure)
		_ = resources.ReleaseResources(ctx, r, options.HostingSystem, true)
	}

	return s, r, err
}

func requireFeatures(tb testing.TB, features ...string) {
	tb.Helper()
	require.Features(tb, flagFeatures.S, features...)
}

func getContainerdOptions() testctrd.ContainerdClientOptions {
	return testctrd.ContainerdClientOptions{
		Address:   *flagContainerdAddress,
		Namespace: *flagContainerdNamespace,
	}
}

func newContainerdClient(ctx context.Context, tb testing.TB) (context.Context, context.CancelFunc, *containerd.Client) {
	tb.Helper()
	return getContainerdOptions().NewClient(ctx, tb)
}

func defaultLCOWOptions(tb testing.TB) *uvm.OptionsLCOW {
	tb.Helper()
	opts := testuvm.DefaultLCOWOptions(tb, cleanName(tb.Name()), hcsOwner)
	if p := *flagLinuxBootFilesPath; p != "" {
		opts.BootFilesPath = p
	}
	return opts
}

//nolint:deadcode,unused // will be used when WCOW tests are updated
func defaultWCOWOptions(tb testing.TB) *uvm.OptionsWCOW {
	tb.Helper()
	opts := uvm.NewDefaultOptionsWCOW(cleanName(tb.Name()), hcsOwner)
	return opts
}

var _nameRegex = regexp.MustCompile(`[\\\/\s]`)

func cleanName(n string) string {
	return _nameRegex.ReplaceAllString(n, "")
}
